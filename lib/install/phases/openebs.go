/*
Copyright 2019 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package phases

import (
	"context"

	"github.com/gravitational/gravity/lib/constants"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/fsm"
	"github.com/gravitational/gravity/lib/ops"
	"github.com/gravitational/gravity/lib/storage"

	"github.com/gravitational/rigging"
	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func NewOpenEBS(p fsm.ExecutorParams, operator ops.Operator, client *kubernetes.Clientset) (fsm.PhaseExecutor, error) {
	logger := &fsm.Logger{
		FieldLogger: logrus.WithField(constants.FieldPhase, p.Phase.ID),
		Key:         opKey(p.Plan),
		Operator:    operator,
	}
	return &systemResources{
		FieldLogger:    logger,
		ExecutorParams: p,
		Client:         client,
	}, nil
}

type openebs struct {
	// FieldLogger is used for logging.
	logrus.FieldLogger
	// ExecutorParams contains common executor parameters.
	fsm.ExecutorParams
	// Client is the cluster Kubernetes client.
	Client *kubernetes.Clientset
}

// Execute creates system Kubernetes resources.
func (r *openebs) Execute(ctx context.Context) error {
	r.Progress.NextStep("Creating OpenEBS configuration")
	r.Info("Creating OpenEBS configuration.")
	ndmConfig := storage.DefaultNDMConfig()
	if r.Phase.Data.Install.PersistentStorage != nil {
		ndmConfig.Apply(r.Phase.Data.Install.PersistentStorage)
	}
	configMap, err := ndmConfig.ToConfigMap()
	if err != nil {
		return trace.Wrap(err)
	}
	_, err = r.Client.CoreV1().Namespaces().Create(&v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaults.OpenEBSNamespace,
		},
	})
	if err != nil {
		return rigging.ConvertError(err)
	}
	_, err = r.Client.CoreV1().ConfigMaps(defaults.OpenEBSNamespace).Create(configMap)
	if err != nil {
		return rigging.ConvertError(err)
	}
	return nil
}

// Rollback deletes created system Kubernetes resources.
func (r *openebs) Rollback(context.Context) error {
	r.Progress.NextStep("Deleting OpenEBS configuration")
	r.Info("Deleting OpenEBS configuration.")
	err := r.Client.CoreV1().ConfigMaps(defaults.OpenEBSNamespace).Delete(constants.OpenEBSNDMMap, &metav1.DeleteOptions{})
	err = rigging.ConvertError(err)
	if err != nil && !trace.IsNotFound(err) {
		return trace.Wrap(err)
	}
	return nil
}

// PreCheck is no-op for this phase.
func (*openebs) PreCheck(context.Context) error { return nil }

// PostCheck is no-op for this phase.
func (*openebs) PostCheck(context.Context) error { return nil }
