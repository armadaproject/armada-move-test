package scheduler

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/hashicorp/go-memdb"
	"github.com/pkg/errors"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	v1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/G-Research/armada/pkg/api"
)

// NodeDb is the scheduler-internal system for storing node information;
// it's used to efficiently find nodes on which a job can be scheduled.
type NodeDb struct {
	// Allowed priorities in sorted order.
	// The assumption is that the number of distinct priorities is small.
	priorities []int32
	// Total amount of resources, e.g., "cpu", "memory", "gpu", managed by the scheduler.
	// Computed approximately by periodically scanning all nodes in the db.
	totalResources map[string]*resource.Quantity
	// Set of node types for which there exists at least 1 node in the db.
	NodeTypes map[string]*NodeType
	// In-memory database. Stores *SchedulerNode.
	Db *memdb.MemDB
	// Resources allocated by the scheduler to in-flight jobs,
	// i.e., jobs for which resource usage is not yet reported by the executor.
	AssignedByNode map[string]AssignedByPriorityAndResourceType
	// Map from job id to the set of nodes on which that job has been assigned resources.
	NodesByJob map[uuid.UUID]map[string]interface{}
	// Map from node id to the set of jobs that have resourced assigned to them on that node.
	JobsByNode map[string]map[uuid.UUID]interface{}
}

// This thing should be responsible for binding pods to nodes.
// So I give it a set of pod and it should atomically bind all those or none of those.
// This will work to remember which resources were promised away already.
// I also need logic to clear that as I get data back from Kubernetes.
// The way to do that is to remember what resources on what node were given to each job.
// Because then I can clear the map when I hear back from the node.
// I could remember which nodes are assigned to each job and then clear it once there are no pending jobs on a node.
// That's the easiest. So let's do that.
// That'd require that I remember which node

// Each node has a set of job ids.
// I keep a map from job id to slice of node names.
// Whenever a job starts running, I remove that job id from all those nodes.
// If the list of job ids becomes empty, I clear the assigned resources for it.

// SelectAndBindNodesForPods takes a slice of pods.
// All pods are either bound to nodes, or an error is returned.
func (nodeDb *NodeDb) SelectAndBindNodesForPods(reqs []*PodSchedulingRequirements) (*SchedulerNode, error) {

	// Find a node at a time.
	// Remember allocated resources within this function.
	// Then record allocated resources at a higher level.
	// This thing can also cause preemptions.
	//

	return nil, nil
}

// SelectAndBindNodeToPod selects a node on which the pod can be scheduled,
// and updates the internal state of the db to indicate that this pod is bound to that node.
func (nodeDb *NodeDb) SelectAndBindNodeToPod(jobId uuid.UUID, req *PodSchedulingRequirements) (*SchedulerNode, error) {

	// Collect all node types that could schedule the pod.
	nodeTypes := nodeDb.NodeTypesMatchingPod(req)

	// The dominant resource is the one for which the pod requests
	// the largest fraction of available resources.
	// For efficiency, the scheduler only considers nodes with enough of the dominant resource.
	dominantResourceType := nodeDb.dominantResource(req)

	// Iterate over candidate nodes.
	txn := nodeDb.Db.Txn(false)
	it, err := NewNodeTypesResourceIterator(
		txn,
		dominantResourceType,
		req.Priority,
		nodeTypes,
		req.ResourceRequirements[dominantResourceType],
	)
	if err != nil {
		return nil, err
	}

	for obj := it.Next(); obj != nil; obj = it.Next() {
		node := obj.(*SchedulerNode)
		if node == nil {
			break
		}
		// TODO: Use the score when selecting a node.
		_, err := node.canSchedulePod(req, nodeDb.AssignedByNode[node.Id])
		if err != nil {
			// fmt.Printf("Can't schedule (score %d): %v -> %s\n", score, node, err)
			continue
		}
		// fmt.Printf("Can schedule (score %d): %v\n", score, node)

		nodeDb.JobsByNode[node.Id][jobId] = true
		nodeDb.NodesByJob[jobId][node.Id] = true
		if assigned, ok := nodeDb.AssignedByNode[node.Id]; ok {
			// assigned.Add()
		} else {
			// NewAssignedByPriorityAndResourceType()
			// nodeDb.AssignedByNode[node.Id] = req
			fmt.Println(assigned)
		}

		return node, nil
	}

	// TODO: Return a more specific reason.
	return nil, errors.New("pod currently not schedulable on any node")
}

// NodeTypesMatchingPod returns a slice composed of all node types
// a given pod could be scheduled on, i.e., all node types with
// matching node selectors and no untolerated taints.
func (nodeDb *NodeDb) NodeTypesMatchingPod(req *PodSchedulingRequirements) []*NodeType {
	rv := make([]*NodeType, 0)
	for _, nodeType := range nodeDb.NodeTypes {
		if err := nodeType.canSchedulePod(req); err != nil {
			rv = append(rv, nodeType)
		}
	}
	return rv
}

func (nodeDb *NodeDb) dominantResource(req *PodSchedulingRequirements) string {
	dominantResourceType := ""
	dominantResourceFraction := 0.0
	for t, q := range req.ResourceRequirements {
		available, ok := nodeDb.totalResources[t]
		if !ok {
			return t
		}
		f := q.AsApproximateFloat64() / available.AsApproximateFloat64()
		if f >= dominantResourceFraction {
			dominantResourceType = t
			dominantResourceFraction = f
		}
	}
	return dominantResourceType
}

// MarkJobRunning notifies the node db that this job is now running.
// When the nodes were bound to the job, resources on those nodes were marked as assigned in the node db.
// When the job is running, those resources are accounted for by the executor,
// and should no longer be marked as assigned in the node db.
func (nodeDb *NodeDb) MarkJobRunning(jobId uuid.UUID) {
	for nodeId := range nodeDb.NodesByJob[jobId] {
		delete(nodeDb.JobsByNode[nodeId], jobId)
		if len(nodeDb.JobsByNode[nodeId]) == 0 {
			delete(nodeDb.AssignedByNode, nodeId)
		}
	}
	delete(nodeDb.NodesByJob, jobId)
}

// SchedulerNode is a scheduler-specific representation of a node.
type SchedulerNode struct {
	// Unique name associated with the node.
	// Only used internally by the scheduler.
	Id string
	// The node type captures scheduling requirements of the node;
	// it's computed from the taints and labels associated with the node.
	NodeType *NodeType
	// We store the NodeType.id here to simplify indexing.
	NodeTypeId string
	// Node info object received from the executor.
	NodeInfo *api.NodeInfo
	// Resources available for jobs of a given priority.
	// E.g., AvailableResources[5]["cpu"] is the amount of CPU available to jobs with priority 5,
	// where available resources = unused resources + resources assigned to lower-priority jobs.
	AvailableResources AvailableByPriorityAndResourceType
	// // Kubernetes node object.
	// Node *v1.Node
}

func (node *SchedulerNode) GetLabels() map[string]string {
	if node.NodeInfo == nil {
		return nil
	}
	return node.NodeInfo.Labels
}

func (node *SchedulerNode) GetTaints() []v1.Taint {
	if node.NodeInfo == nil {
		return nil
	}
	return node.NodeInfo.Taints
}

type QuantityByPriorityAndResourceType map[int32]map[string]resource.Quantity

// AvailableByPriorityAndResourceType accounts for resources available to pods of a given priority.
// E.g., AvailableByPriorityAndResourceType[5]["cpu"] is the amount of CPU available to pods with priority 5,
// where available resources = unused resources + resources assigned to lower-priority pods.
type AvailableByPriorityAndResourceType QuantityByPriorityAndResourceType

// AssignedByPriorityAndResourceType accounts for resources assigned to pods of a given priority or higher.
// E.g., AssignedByPriorityAndResourceType[5]["cpu"] is the amount of CPU assigned to pods with priority 5 or higher.
type AssignedByPriorityAndResourceType QuantityByPriorityAndResourceType

func (availableByPriorityAndResourceType AvailableByPriorityAndResourceType) Get(priority int32, resourceType string) resource.Quantity {
	if availableByPriorityAndResourceType == nil {
		return resource.MustParse("0")
	}
	quantityByResourceType, ok := availableByPriorityAndResourceType[priority]
	if !ok {
		return resource.MustParse("0")
	}
	q, ok := quantityByResourceType[resourceType]
	if !ok {
		return resource.MustParse("0")
	}
	return q
}

func (assignedByPriorityAndResourceType AssignedByPriorityAndResourceType) Get(priority int32, resourceType string) resource.Quantity {
	if assignedByPriorityAndResourceType == nil {
		return resource.MustParse("0")
	}
	quantityByResourceType, ok := assignedByPriorityAndResourceType[priority]
	if !ok {
		return resource.MustParse("0")
	}
	q, ok := quantityByResourceType[resourceType]
	if !ok {
		return resource.MustParse("0")
	}
	return q
}

func (nodeItem *SchedulerNode) availableQuantityByPriorityAndResource(priority int32, resourceType string) resource.Quantity {
	return nodeItem.AvailableResources.Get(priority, resourceType)
}

func NewNodeDb(priorities []int32, resourceTypes []string) (*NodeDb, error) {
	db, err := memdb.NewMemDB(nodeDbSchema(priorities, resourceTypes))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	priorities = []int32(priorities)
	totalResources := make(map[string]*resource.Quantity)
	for _, resourceType := range resourceTypes {
		q := resource.MustParse("0")
		totalResources[resourceType] = &q
	}
	slices.Sort(priorities)
	return &NodeDb{
		priorities:     priorities,
		NodeTypes:      make(map[string]*NodeType),
		totalResources: totalResources,
		Db:             db,
	}, nil
}

// Upsert will update the node db with the given nodes.
func (nodeDb *NodeDb) Upsert(nodes []*SchedulerNode) error {
	maxPriority := nodeDb.priorities[len(nodeDb.priorities)-1]
	txn := nodeDb.Db.Txn(true)
	defer txn.Abort()
	for _, node := range nodes {

		// TODO: Add resources for every insertion. This will overestimate resources significantly.
		m := node.AvailableResources[maxPriority]
		for t, q := range m {
			available := nodeDb.totalResources[t]
			if available == nil {
				nodeDb.totalResources[t] = &q
			} else {
				available.Add(q)
				nodeDb.totalResources[t] = available
			}
		}

		err := txn.Insert("nodes", node)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	txn.Commit()

	// Record all known node types.
	for _, node := range nodes {
		nodeDb.NodeTypes[node.NodeType.id] = node.NodeType
	}

	return nil
}

func (nodeDb *NodeDb) SchedulerNodeFromNodeInfo(nodeInfo *api.NodeInfo, executor string) *SchedulerNode {
	return &SchedulerNode{
		Id:       fmt.Sprintf("%s-%s", executor, nodeInfo.Name),
		NodeType: NewNodeTypeFromNodeInfo(nodeInfo, nil, nil),
		NodeInfo: nodeInfo,
		// Node:               nodeFromNodeInfo(nodeInfo),
		AvailableResources: availableResourcesFromNodeInfo(nodeInfo, nodeDb.priorities),
	}
}

func availableResourcesFromNodeInfo(nodeInfo *api.NodeInfo, allowedPriorities []int32) map[int32]map[string]resource.Quantity {
	rv := make(map[int32]map[string]resource.Quantity)
	for _, priority := range allowedPriorities {
		rv[priority] = maps.Clone(nodeInfo.TotalResources)
	}
	for allocatedPriority, allocatedResources := range nodeInfo.AllocatedResources {
		for _, priority := range allowedPriorities {
			if priority <= allocatedPriority {
				for resource, quantity := range allocatedResources.Resources {
					q := rv[priority][resource]
					q.Sub(quantity)
					rv[priority][resource] = q
				}
			}
		}
	}
	return rv
}

func nodeFromNodeInfo(nodeInfo *api.NodeInfo) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: nodeInfo.GetLabels(),
		},
		Spec: v1.NodeSpec{
			Taints: nodeInfo.GetTaints(),
		},
	}
}

func nodeDbSchema(priorities []int32, resources []string) *memdb.DBSchema {
	indexes := make(map[string]*memdb.IndexSchema)
	indexes["id"] = &memdb.IndexSchema{
		Name:    "id",
		Unique:  true,
		Indexer: &memdb.StringFieldIndex{Field: "Id"},
	}
	for _, priority := range priorities {
		for _, resource := range resources {
			name := fmt.Sprintf("%d-%s", priority, resource)
			indexes[name] = &memdb.IndexSchema{
				Name:   name,
				Unique: false,
				Indexer: &memdb.CompoundIndex{
					Indexes: []memdb.Indexer{
						&memdb.StringFieldIndex{Field: "NodeTypeId"},
						&NodeItemAvailableResourceIndex{
							Resource: resource,
							Priority: priority,
						},
					},
				},
			}
		}
	}
	return &memdb.DBSchema{
		Tables: map[string]*memdb.TableSchema{
			"nodes": {
				Name:    "nodes",
				Indexes: indexes,
			},
		},
	}
}

func nodeResourcePriorityIndexName(resource string, priority int32) string {
	return fmt.Sprintf("%d-%s", priority, resource)
}