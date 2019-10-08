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

	"github.com/gravitational/gravity/lib/constants"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/ops"
	"github.com/gravitational/gravity/lib/storage"

	"github.com/gravitational/rigging"
	"github.com/gravitational/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetPersistentStorage retrieves the current persistent storage configuration.
func (o *Operator) GetPersistentStorage(ctx context.Context, key ops.SiteKey) (storage.PersistentStorage, error) {
	client, err := o.GetKubeClient()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	cm, err := client.CoreV1().ConfigMaps(defaults.OpenEBSNamespace).Get(
		constants.OpenEBSNDMMap, metav1.GetOptions{})
	if err != nil {
		return nil, rigging.ConvertError(err)
	}
	ndmConfig, err := storage.NDMConfigFromConfigMap(cm)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return storage.PersistentStorageFromNDMConfig(ndmConfig), nil
}
