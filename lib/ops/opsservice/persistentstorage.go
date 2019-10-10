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

package opsservice

import (
	"context"
	"fmt"
	"time"

	"github.com/gravitational/gravity/lib/constants"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/ops"
	"github.com/gravitational/gravity/lib/storage"

	"github.com/gravitational/rigging"
	"github.com/gravitational/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

// GetPersistentStorage retrieves the current persistent storage configuration.
func (o *Operator) GetPersistentStorage(ctx context.Context, key ops.SiteKey) (storage.PersistentStorage, error) {
	if o.cfg.OpenEBS == nil {
		return nil, trace.BadParameter("persistent storage is not configured")
	}
	ndmConfig, err := o.cfg.OpenEBS.GetNDMConfig()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return storage.PersistentStorageFromNDMConfig(ndmConfig), nil
}

// UpdatePersistentStorage updates cluster persistent storage configuration.
func (o *Operator) UpdatePersistentStorage(ctx context.Context, req ops.UpdatePersistentStorageRequest) error {
	if o.cfg.OpenEBS == nil {
		return trace.BadParameter("persistent storage is not configured")
	}
	ndmConfig, err := o.cfg.OpenEBS.GetNDMConfig()
	if err != nil {
		return trace.Wrap(err)
	}
	ndmConfig.Apply(req.Resource)
	err = o.cfg.OpenEBS.UpdateNDMConfig(ndmConfig)
	if err != nil {
		return trace.Wrap(err)
	}
	err = o.cfg.OpenEBS.RestartNDM()
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// OpenEBSControl provides interface for managing OpenEBS in the cluster.
type OpenEBSControl interface {
	// GetNDMConfig returns node device manager configuration.
	GetNDMConfig() (*storage.NDMConfig, error)
	// UpdateNDMConfig updates node device manager configuration.
	UpdateNDMConfig(*storage.NDMConfig) error
	// RestartNDM restarts node device manager pods.
	RestartNDM() error
}

type openEBSControl struct {
	// cm is config map interface in the configured namespace.
	cm corev1.ConfigMapInterface
	// ds is daemon set interface in the configured namespace.
	ds appsv1.DaemonSetInterface
	// name is the node device manager config map name.
	name string
}

// OpenEBSConfig is the OpenEBS controller configuration.
type OpenEBSConfig struct {
	// Client is the Kubernetes client.
	Client *kubernetes.Clientset
	// Namespace is the namespace where OpenEBS components reside.
	Namespace string
	// Name is the name of the config map with node device manager configuration.
	Name string
}

// CheckAndSetDefaults validates the config and sets defaults.
func (c *OpenEBSConfig) CheckAndSetDefaults() error {
	if c.Client == nil {
		return trace.BadParameter("missing Kubernetes client")
	}
	if c.Namespace == "" {
		c.Namespace = defaults.OpenEBSNamespace
	}
	if c.Name == "" {
		c.Name = constants.OpenEBSNDMMap
	}
	return nil
}

// NewOpenEBSContol returns a new OpenEBS controller for the provided client.
func NewOpenEBSControl(config OpenEBSConfig) (*openEBSControl, error) {
	if err := config.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &openEBSControl{
		cm:   config.Client.Core().ConfigMaps(config.Namespace),
		ds:   config.Client.Apps().DaemonSets(config.Namespace),
		name: config.Name,
	}, nil
}

// GetNDMConfig returns node device manager configuration.
func (c *openEBSControl) GetNDMConfig() (*storage.NDMConfig, error) {
	cm, err := c.cm.Get(c.name, metav1.GetOptions{})
	if err != nil {
		return nil, rigging.ConvertError(err)
	}
	config, err := storage.NDMConfigFromConfigMap(cm)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return config, nil
}

// UpdateNDMConfig updates node device manager configuration.
func (c *openEBSControl) UpdateNDMConfig(config *storage.NDMConfig) error {
	cm, err = config.ToConfigMap()
	if err != nil {
		return trace.Wrap(err)
	}
	_, err = c.cm.Update(cm)
	if err != nil {
		return rigging.ConvertError(err)
	}
	return nil
}

// RestartNDM restarts node device manager pods.
func (c *openEBSControl) RestartNDM() error {
	_, err := c.ds.Patch(c.name, types.StrategicMergePatchType, formatRestartPatch())
	if err != nil {
		return rigging.ConvertError(err)
	}
	return nil
}

// formatRestartPatch returns the patch that sets the restartedAt annotation
// on the daemon set object.
func formatRestartPatch() []byte {
	return []byte(fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"gravity-site.gravitational.io/restartedAt":"%s"}}}}}`,
		time.Now().Format(constants.HumanDateFormatSeconds)))
}
