"""
Armada Python GRPC Client
"""
from concurrent.futures import ThreadPoolExecutor
import os
from typing import List, Optional
from armada_client.armada import (
    event_pb2,
    event_pb2_grpc,
    usage_pb2_grpc,
    submit_pb2_grpc,
    submit_pb2,
)


class ArmadaClient:
    """
    The Armada Client
    Implementation of gRPC stubs from events, queues and submit

    Attributes:
        channel: gRPC channel
        max_workers: number of cores for thread pools
    gRPC channels is for authentication.
    See https://grpc.github.io/grpc/python/grpc.html
    """

    def __init__(self, channel, max_workers=os.cpu_count()):

        self.executor = ThreadPoolExecutor(max_workers=max_workers or 1)

        self.submit_stub = submit_pb2_grpc.SubmitStub(channel)
        self.event_stub = event_pb2_grpc.EventStub(channel)
        self.usage_stub = usage_pb2_grpc.UsageStub(channel)

    def get_job_events_stream(
        self,
        queue: str,
        job_set_id: str,
        from_message_id: Optional[str] = None,
    ):
        """Implementation of GetJobSetEvents rpc function"""
        jsr = event_pb2.JobSetRequest(
            queue=queue,
            id=job_set_id,
            from_message_id=from_message_id,
            watch=True,
            errorIfMissing=True,
        )
        return self.event_stub.GetJobSetEvents(jsr)

    def submit_jobs(self, queue: str, job_set_id: str, job_request_items):
        """Implementation of SubmitJobs rpc function"""
        request = submit_pb2.JobSubmitRequest(
            queue=queue, job_set_id=job_set_id, job_request_items=job_request_items
        )
        response = self.submit_stub.SubmitJobs(request)
        return response

    def cancel_jobs(
        self,
        queue: Optional[str] = None,
        job_id: Optional[str] = None,
        job_set_id: Optional[str] = None,
    ):
        """Implementation of CancelJobs rpc function"""
        request = submit_pb2.JobCancelRequest(
            queue=queue, job_id=job_id, job_set_id=job_set_id
        )
        response = self.submit_stub.CancelJobs(request)
        return response

    def reprioritize_jobs(
        self,
        new_priority: float,
        job_ids: Optional[List[str]] = None,
        job_set_id: Optional[str] = None,
        queue: Optional[str] = None,
    ):
        """Implementation of ReprioritizeJobs rpc function"""

        request = submit_pb2.JobReprioritizeRequest(
            job_ids=job_ids,
            job_set_id=job_set_id,
            queue=queue,
            new_priority=new_priority,
        )
        response = self.submit_stub.ReprioritizeJobs(request)
        return response

    def create_queue(self, name: str, **queue_params):
        """Implementation of CreateQueue rpc function"""
        request = submit_pb2.Queue(name=name, **queue_params)
        response = self.submit_stub.CreateQueue(request)
        return response

    def update_queue(self, name: str, **queue_params):
        """Implementation of UpdateQueue rpc function"""
        request = submit_pb2.Queue(name=name, **queue_params)
        response = self.submit_stub.UpdateQueue(request)
        return response

    def delete_queue(self, name: str):
        """Implementation of DeleteQueue rpc function"""
        request = submit_pb2.QueueDeleteRequest(name=name)
        response = self.submit_stub.DeleteQueue(request)
        return response

    def get_queue(self, name: str):
        """Impl of GetQueue"""
        request = submit_pb2.QueueGetRequest(name=name)
        response = self.submit_stub.GetQueue(request)
        return response

    def get_queue_info(self, name: str):
        """Impl of GetQueueInfo"""
        request = submit_pb2.QueueInfoRequest(name=name)
        response = self.submit_stub.GetQueueInfo(request)
        return response


def unwatch_events(event_stream):
    """Grpc way to cancel a stream"""
    event_stream.cancel()