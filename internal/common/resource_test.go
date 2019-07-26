package common

import (
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"testing"
)

func TestCalculateTotalResource(t *testing.T) {
	resources := makeDefaultNodeResource()
	node1 := makeNodeWithResource(resources)
	node2 := makeNodeWithResource(resources)

	//Expected is resources * 2 nodes
	expectedResult := FromResourceList(resources)
	expectedResult.Add(expectedResult)

	result := CalculateTotalResource([]*v1.Node{&node1, &node2})
	assert.Equal(t, result, expectedResult)
}

func TestCalculateTotalResourceLimit_ShouldSumAllPods(t *testing.T) {
	resources := makeContainerResource(100, 50)
	pod1 := makePodWithResource([]*v1.ResourceList{&resources}, []*v1.ResourceList{})
	pod2 := makePodWithResource([]*v1.ResourceList{&resources}, []*v1.ResourceList{})

	//Expected is resources * 2 containers
	expectedResult := makeContainerResource(200, 100)

	result := CalculateTotalResourceLimit([]*v1.Pod{&pod1, &pod2})
	assert.Equal(t, result, FromResourceList(expectedResult))
}

func TestTotalResourceLimit_ShouldSumAllContainers(t *testing.T) {
	resources := makeContainerResource(100, 50)
	pod := makePodWithResource([]*v1.ResourceList{&resources, &resources}, []*v1.ResourceList{})

	//Expected is resources * 2 containers
	expectedResult := makeContainerResource(200, 100)

	result := TotalResourceLimit(&pod.Spec)
	assert.Equal(t, result, FromResourceList(expectedResult))
}

func TestTotalResourceLimit_ShouldReportMaxInitContainerValues(t *testing.T) {
	highCpuResource := makeContainerResource(1000, 5)
	highRamResource := makeContainerResource(100, 500)
	//With init containers, it should take the max of each individual resource from all init containers
	expectedResult := makeContainerResource(1000, 500)

	pod := makePodWithResource([]*v1.ResourceList{}, []*v1.ResourceList{&highCpuResource, &highRamResource})

	result := TotalResourceLimit(&pod.Spec)
	assert.Equal(t, result, FromResourceList(expectedResult))
}

func TestTotalResourceLimit_ShouldCombineMaxInitContainerResourcesWithSummedContainerResources(t *testing.T) {
	standardResource := makeContainerResource(100, 50)
	highCpuResource := makeContainerResource(1000, 50)

	pod := makePodWithResource([]*v1.ResourceList{&standardResource, &standardResource}, []*v1.ResourceList{&standardResource, &highCpuResource})
	//It should sum the containers and compare value to each init container, taking the max
	//Cpu is 1000, as the sum of the two containers is 200, which is lower than the max of any given init container (1000)
	//Memory is 100, as the sum of the two containers is 100, which is higher than the max of any given init container (both init containers are 50 each)
	expectedResult := makeContainerResource(1000, 100)

	result := TotalResourceLimit(&pod.Spec)
	assert.Equal(t, result, FromResourceList(expectedResult))
}

func makeDefaultNodeResource() v1.ResourceList {
	cpuResource := resource.NewQuantity(100, resource.DecimalSI)
	memoryResource := resource.NewQuantity(50*1024*1024*1024, resource.DecimalSI)
	storageResource := resource.NewQuantity(500*1024*1024*1024, resource.DecimalSI)
	ephemeralStorageResource := resource.NewQuantity(20*1024*1024*1024, resource.DecimalSI)
	resourceMap := map[v1.ResourceName]resource.Quantity{
		v1.ResourceCPU:              *cpuResource,
		v1.ResourceMemory:           *memoryResource,
		v1.ResourceStorage:          *storageResource,
		v1.ResourceEphemeralStorage: *ephemeralStorageResource,
	}
	return resourceMap
}

func makeContainerResource(cores int64, gigabytesRam int64) v1.ResourceList {
	cpuResource := resource.NewQuantity(cores, resource.DecimalSI)
	memoryResource := resource.NewQuantity(gigabytesRam*1024*1024*1024, resource.DecimalSI)
	resourceMap := map[v1.ResourceName]resource.Quantity{
		v1.ResourceCPU:    *cpuResource,
		v1.ResourceMemory: *memoryResource,
	}
	return resourceMap
}

func makePodWithResource(containerResources []*v1.ResourceList, initContainerResources []*v1.ResourceList) v1.Pod {
	containers := make([]v1.Container, len(containerResources))
	for i, res := range containerResources {
		containers[i] = v1.Container{
			Resources: v1.ResourceRequirements{
				Limits: *res,
			},
		}
	}

	initContainers := make([]v1.Container, len(initContainerResources))
	for i, res := range initContainerResources {
		initContainers[i] = v1.Container{
			Resources: v1.ResourceRequirements{
				Limits: *res,
			},
		}
	}

	pod := v1.Pod{
		Spec: v1.PodSpec{
			Containers:     containers,
			InitContainers: initContainers,
		},
	}

	return pod
}

func makeNodeWithResource(resources map[v1.ResourceName]resource.Quantity) v1.Node {
	node := v1.Node{
		Status: v1.NodeStatus{
			Allocatable: resources,
		},
	}
	return node
}
