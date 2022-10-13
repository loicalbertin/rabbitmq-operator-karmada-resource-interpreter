package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	rabbitv1beta1 "github.com/rabbitmq/cluster-operator/api/v1beta1"

	configv1alpha1 "github.com/karmada-io/karmada/pkg/apis/config/v1alpha1"
	"github.com/karmada-io/karmada/pkg/webhook/interpreter"
)

// Check if our rabbitClusterInterpreter implements necessary interface
var _ interpreter.Handler = &rabbitClusterInterpreter{}
var _ interpreter.DecoderInjector = &rabbitClusterInterpreter{}

// rabbitClusterInterpreter explore resource with request operation.
type rabbitClusterInterpreter struct {
	decoder *interpreter.Decoder
}

// Handle implements interpreter.Handler interface.
// It yields a response to an ExploreRequest.
func (e *rabbitClusterInterpreter) Handle(ctx context.Context, req interpreter.Request) interpreter.Response {
	rabbitCluster := &rabbitv1beta1.RabbitmqCluster{}
	err := e.decoder.Decode(req, rabbitCluster)
	if err != nil {
		return interpreter.Errored(http.StatusBadRequest, err)
	}

	klog.Infof("Explore rabbitCluster(%s/%s) for request: %s", rabbitCluster.GetNamespace(), rabbitCluster.GetName(), req.Operation)

	switch req.Operation {
	case configv1alpha1.InterpreterOperationInterpretReplica:
		return e.responseWithExploreReplica(rabbitCluster)
	case configv1alpha1.InterpreterOperationReviseReplica:
		return e.responseWithExploreReviseReplica(rabbitCluster, req)
	case configv1alpha1.InterpreterOperationRetain:
		return e.responseWithExploreRetaining(rabbitCluster, req)
	case configv1alpha1.InterpreterOperationAggregateStatus:
		return e.responseWithExploreAggregateStatus(rabbitCluster, req)
	// case configv1alpha1.InterpreterOperationInterpretHealth:
	// 	return e.responseWithExploreInterpretHealth(rabbitCluster)
	// case configv1alpha1.InterpreterOperationInterpretStatus:
	// 	return e.responseWithExploreInterpretStatus(rabbitCluster)
	default:
		return interpreter.Errored(http.StatusBadRequest, fmt.Errorf("wrong request operation type: %s", req.Operation))
	}
}

// InjectDecoder implements interpreter.DecoderInjector interface.
func (e *rabbitClusterInterpreter) InjectDecoder(d *interpreter.Decoder) {
	e.decoder = d
}

func (e *rabbitClusterInterpreter) responseWithExploreReplica(rabbit *rabbitv1beta1.RabbitmqCluster) interpreter.Response {
	res := interpreter.Succeeded("")
	res.Replicas = rabbit.Spec.Replicas
	return res
}

func (e *rabbitClusterInterpreter) responseWithExploreReviseReplica(rabbit *rabbitv1beta1.RabbitmqCluster, req interpreter.Request) interpreter.Response {
	wantedRabbit := rabbit.DeepCopy()
	wantedRabbit.Spec.Replicas = req.DesiredReplicas
	marshaledBytes, err := json.Marshal(wantedRabbit)
	if err != nil {
		return interpreter.Errored(http.StatusInternalServerError, err)
	}
	return interpreter.PatchResponseFromRaw(req.Object.Raw, marshaledBytes)
}

func (e *rabbitClusterInterpreter) responseWithExploreRetaining(desiredRabbit *rabbitv1beta1.RabbitmqCluster, req interpreter.Request) interpreter.Response {
	if req.ObservedObject == nil {
		err := fmt.Errorf("nil observedObject in exploreReview with operation type: %s", req.Operation)
		return interpreter.Errored(http.StatusBadRequest, err)
	}
	observerRabbitCluster := &rabbitv1beta1.RabbitmqCluster{}
	err := e.decoder.DecodeRaw(*req.ObservedObject, observerRabbitCluster)
	if err != nil {
		return interpreter.Errored(http.StatusBadRequest, err)
	}

	// We need to retain the `.spec.image` field of the actual observed rabbitCluster object in member cluster,
	// and prevent from being overwritten by karmada controller-plane. Otherwise we run into an infinite loop of both
	// Karmada & RabbitMQ operator updating this field.
	wantedRabbitCluster := desiredRabbit.DeepCopy()
	wantedRabbitCluster.Spec.Image = observerRabbitCluster.Spec.Image

	marshaledBytes, err := json.Marshal(wantedRabbitCluster)
	if err != nil {
		return interpreter.Errored(http.StatusInternalServerError, err)
	}
	return interpreter.PatchResponseFromRaw(req.Object.Raw, marshaledBytes)
}

func (e *rabbitClusterInterpreter) responseWithExploreAggregateStatus(rabbitCluster *rabbitv1beta1.RabbitmqCluster, req interpreter.Request) interpreter.Response {
	wantedRabbitCluster := rabbitCluster.DeepCopy()
	var readyReplicas int32
	for _, item := range req.AggregatedStatus {
		if item.Status == nil {
			continue
		}
		status := &rabbitv1beta1.RabbitmqClusterStatus{}
		if err := json.Unmarshal(item.Status.Raw, status); err != nil {
			return interpreter.Errored(http.StatusInternalServerError, err)
		}
		for _, rmqCondition := range status.Conditions {
			if rmqCondition.Type == "AllReplicasReady" && rmqCondition.Status == corev1.ConditionTrue {
				readyReplicas += 1
			}
		}

	}
	if readyReplicas == *req.DesiredReplicas {
		wantedRabbitCluster.Status.SetCondition("AllReplicasReady", corev1.ConditionTrue, "aggregated status", "True")
	} else {
		wantedRabbitCluster.Status.SetCondition("AllReplicasReady", corev1.ConditionFalse, "aggregated status", "False")
	}
	marshaledBytes, err := json.Marshal(wantedRabbitCluster)
	if err != nil {
		return interpreter.Errored(http.StatusInternalServerError, err)
	}
	return interpreter.PatchResponseFromRaw(req.Object.Raw, marshaledBytes)
}

// func (e *rabbitClusterInterpreter) responseWithExploreInterpretHealth(rabbitCluster *rabbitv1beta1.RabbitmqCluster) interpreter.Response {
// 	healthy := pointer.Bool(false)
// 	if rabbitCluster.Status.ReadyReplicas == *rabbitCluster.Spec.Replicas {
// 		healthy = pointer.Bool(true)
// 	}

// 	res := interpreter.Succeeded("")
// 	res.Healthy = healthy
// 	return res
// }

// func (e *rabbitClusterInterpreter) responseWithExploreInterpretStatus(rabbitCluster *rabbitv1beta1.RabbitmqCluster) interpreter.Response {
// 	status := rabbitv1beta1.RabbitmqClusterStatus{
// 		ReadyReplicas: rabbitCluster.Status.ReadyReplicas,
// 	}

// 	marshaledBytes, err := json.Marshal(status)
// 	if err != nil {
// 		return interpreter.Errored(http.StatusInternalServerError, err)
// 	}

// 	res := interpreter.Succeeded("")
// 	res.RawStatus = &runtime.RawExtension{
// 		Raw: marshaledBytes,
// 	}

// 	return res
// }
