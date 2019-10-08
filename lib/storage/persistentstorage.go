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

package storage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gravitational/gravity/lib/constants"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/jonboulle/clockwork"

	"github.com/gravitational/teleport/lib/services"
	teleservices "github.com/gravitational/teleport/lib/services"
	"github.com/gravitational/teleport/lib/utils"
	"github.com/gravitational/trace"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PersistentStorage interface {
	services.Resource
	CheckAndSetDefaults() error
	GetMountExcludes() []string
	GetVendorIncludes() []string
	GetVendorExcludes() []string
	GetDeviceIncludes() []string
	GetDeviceExcludes() []string
}

func NewPersistentStorage(spec PersistentStorageSpecV1) PersistentStorage {
	return &PersistentStorageV1{
		Kind:    KindPersistentStorage,
		Version: services.V1,
		Metadata: services.Metadata{
			Name:      KindPersistentStorage,
			Namespace: defaults.Namespace,
		},
		Spec: spec,
	}
}

func PersistentStorageFromNDMConfig(c *NDMConfig) PersistentStorage {
	return NewPersistentStorage(PersistentStorageSpecV1{
		OpenEBS: OpenEBS{
			Filters: OpenEBSFilters{
				MountPoints: OpenEBSFilter{
					Exclude: c.MountExcludes(),
				},
				Vendors: OpenEBSFilter{
					Exclude: c.VendorExcludes(),
					Include: c.VendorIncludes(),
				},
				Devices: OpenEBSFilter{
					Exclude: c.DeviceExcludes(),
					Include: c.DeviceIncludes(),
				},
			},
		},
	})
}

func DefaultPersistentStorage() PersistentStorage {
	ps := &PersistentStorageV1{
		Kind:    KindPersistentStorage,
		Version: services.V1,
	}
	ps.CheckAndSetDefaults()
	return ps
}

type PersistentStorageV1 struct {
	Kind     string                  `json:"kind"`
	Version  string                  `json:"version"`
	Metadata services.Metadata       `json:"metadata"`
	Spec     PersistentStorageSpecV1 `json:"spec"`
}

type PersistentStorageSpecV1 struct {
	OpenEBS OpenEBS `json:"openebs"`
}

type OpenEBS struct {
	Filters OpenEBSFilters `json:"filters"`
}

type OpenEBSFilters struct {
	MountPoints OpenEBSFilter `json:"mountPoints"`
	Vendors     OpenEBSFilter `json:"vendors"`
	Devices     OpenEBSFilter `json:"devices"`
}

type OpenEBSFilter struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

// GetName returns the resource name.
func (ps *PersistentStorageV1) GetName() string {
	return ps.Metadata.Name
}

// SetName sets the resource name.
func (ps *PersistentStorageV1) SetName(name string) {
	ps.Metadata.Name = name
}

// GetMetadata returns the resource metadata.
func (ps *PersistentStorageV1) GetMetadata() teleservices.Metadata {
	return ps.Metadata
}

// SetExpiry sets the resource expiration time.
func (ps *PersistentStorageV1) SetExpiry(expires time.Time) {
	ps.Metadata.SetExpiry(expires)
}

// Expires returns the resource expiration time.
func (ps *PersistentStorageV1) Expiry() time.Time {
	return ps.Metadata.Expiry()
}

// SetTTL sets the resource TTL.
func (ps *PersistentStorageV1) SetTTL(clock clockwork.Clock, ttl time.Duration) {
	ps.Metadata.SetTTL(clock, ttl)
}

func (ps *PersistentStorageV1) GetMountExcludes() []string {
	return ps.Spec.OpenEBS.Filters.MountPoints.Exclude
}

func (ps *PersistentStorageV1) GetVendorIncludes() []string {
	return ps.Spec.OpenEBS.Filters.Vendors.Include
}

func (ps *PersistentStorageV1) GetVendorExcludes() []string {
	return ps.Spec.OpenEBS.Filters.Vendors.Exclude
}

func (ps *PersistentStorageV1) GetDeviceIncludes() []string {
	return ps.Spec.OpenEBS.Filters.Devices.Include
}

func (ps *PersistentStorageV1) GetDeviceExcludes() []string {
	return ps.Spec.OpenEBS.Filters.Devices.Exclude
}

func (ps *PersistentStorageV1) CheckAndSetDefaults() error {
	if ps.Metadata.Name == "" {
		ps.Metadata.Name = KindPersistentStorage
	}
	if err := ps.Metadata.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	// TODO: Append these instead?
	if len(ps.Spec.OpenEBS.Filters.MountPoints.Exclude) == 0 {
		ps.Spec.OpenEBS.Filters.MountPoints.Exclude = []string{"/", "/etc/hosts", "/boot"}
	}
	if len(ps.Spec.OpenEBS.Filters.Vendors.Exclude) == 0 {
		ps.Spec.OpenEBS.Filters.Vendors.Exclude = []string{"CLOUDBYT", "OpenEBS"}
	}
	if len(ps.Spec.OpenEBS.Filters.Devices.Exclude) == 0 {
		ps.Spec.OpenEBS.Filters.Devices.Exclude = []string{"loop", "/dev/fd0", "/dev/sr0", "/dev/ram", "/dev/dm-", "/dev/md"}
	}
	return nil
}

func UnmarshalPersistentStorage(data []byte) (PersistentStorage, error) {
	jsonData, err := utils.ToJSON(data)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	var header services.ResourceHeader
	if err := json.Unmarshal(jsonData, &header); err != nil {
		return nil, trace.Wrap(err)
	}
	switch header.Version {
	case services.V1:
		var ps PersistentStorageV1
		err := utils.UnmarshalWithSchema(GetPersistentStorageSchema(), &ps, jsonData)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if err := ps.CheckAndSetDefaults(); err != nil {
			return nil, trace.Wrap(err)
		}
		return &ps, nil
	}
	return nil, trace.BadParameter("%v resource version %q is not supported",
		KindPersistentStorage, header.Version)
}

func MarshalPersistentStorage(ps PersistentStorage, opts ...services.MarshalOption) ([]byte, error) {
	return json.Marshal(ps)
}

func GetPersistentStorageSchema() string {
	return fmt.Sprintf(services.V2SchemaTemplate, MetadataSchema,
		PersistentStorageSpecV1Schema, "")
}

var PersistentStorageSpecV1Schema = `{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "openebs": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "filters": {
          "type": "object",
          "additionalProperties": false,
          "properties": {
            "mountPoints": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "exclude": {"type": "array", "items": {"type": "string"}},
              }
            },
            "vendors": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "include": {"type": "array", "items": {"type": "string"}},
                "exclude": {"type": "array", "items": {"type": "string"}},
              }
            },
            "devices": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "include": {"type": "array", "items": {"type": "string"}},
                "exclude": {"type": "array", "items": {"type": "string"}},
              }
            }
          }
        }
      }
    }
  }
}`

type NDMConfig struct {
	ProbeConfigs  []NDMProbe  `yaml:"probeconfigs"`
	FilterConfigs []NDMFilter `yaml:"filterconfigs"`
}

func (c *NDMConfig) getFilter(key string) *NDMFilter {
	for _, filter := range c.FilterConfigs {
		if filter.Key == key {
			return &filter
		}
	}
	return &NDMFilter{}
}

func (c *NDMConfig) MountExcludes() []string {
	return strings.Split(c.getFilter("os-disk-exclude-filter").Exclude, ",")
}

func (c *NDMConfig) SetMountExcludes(excludes []string) {
	c.getFilter("os-disk-exclude-filter").Exclude = strings.Join(excludes, ",")
}

func (c *NDMConfig) VendorExcludes() []string {
	return strings.Split(c.getFilter("vendor-filter").Exclude, ",")
}

func (c *NDMConfig) SetVendorExcludes(excludes []string) {
	c.getFilter("vendor-filter").Exclude = strings.Join(excludes, ",")
}

func (c *NDMConfig) VendorIncludes() []string {
	return strings.Split(c.getFilter("vendor-filter").Include, ",")
}

func (c *NDMConfig) SetVendorIncludes(includes []string) {
	c.getFilter("vendor-filter").Include = strings.Join(includes, ",")
}

func (c *NDMConfig) DeviceExcludes() []string {
	return strings.Split(c.getFilter("path-filter").Exclude, ",")
}

func (c *NDMConfig) SetDeviceExcludes(excludes []string) {
	c.getFilter("path-filter").Exclude = strings.Join(excludes, ",")
}

func (c *NDMConfig) DeviceIncludes() []string {
	return strings.Split(c.getFilter("path-filter").Include, ",")
}

func (c *NDMConfig) SetDeviceIncludes(includes []string) {
	c.getFilter("path-filter").Include = strings.Join(includes, ",")
}

func DefaultNDMConfig() *NDMConfig {
	return &NDMConfig{
		ProbeConfigs: []NDMProbe{
			{Name: "udev probe", Key: "udev-probe", State: true},
			{Name: "searchest probe", Key: "searchest-probe", State: false},
			{Name: "smart probe", Key: "smart-probe", State: true},
		},
		FilterConfigs: []NDMFilter{
			{
				Name:    "os disk exclude filter",
				Key:     "os-disk-exclude-filter",
				State:   true,
				Exclude: strings.Join(defaultMountExcludes, ","),
			},
			{
				Name:    "vendor filter",
				Key:     "vendor-filter",
				State:   true,
				Exclude: strings.Join(defaultVendorExcludes, ","),
			},
			{
				Name:    "path filter",
				Key:     "path-filter",
				State:   true,
				Exclude: strings.Join(defaultDeviceExcludes, ","),
			},
		},
	}
}

func NDMConfigFromConfigMap(cm *v1.ConfigMap) (*NDMConfig, error) {
	data, ok := cm.Data["node-disk-manager.config"]
	if !ok || len(data) == 0 {
		return nil, trace.BadParameter("config map %v does not contain node disk manager configuration", cm.Name)
	}
	var config NDMConfig
	if err := yaml.Unmarshal([]byte(data), &config); err != nil {
		return nil, trace.Wrap(err)
	}
	return &config, nil
}

// Apply applies parameters from the provided resource to this configuration.
func (c *NDMConfig) Apply(ps PersistentStorage) {
	c.SetMountExcludes(ps.GetMountExcludes())
	c.SetVendorIncludes(ps.GetVendorIncludes())
	c.SetVendorExcludes(ps.GetVendorExcludes())
	c.SetDeviceIncludes(ps.GetDeviceIncludes())
	c.SetDeviceExcludes(ps.GetDeviceExcludes())
}

func (c *NDMConfig) ToConfigMap() (*v1.ConfigMap, error) {
	data, err := yaml.Marshal(c)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       constants.KindConfigMap,
			APIVersion: metav1.SchemeGroupVersion.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.OpenEBSNDMMap,
			Namespace: defaults.OpenEBSNamespace,
			Labels: map[string]string{
				"openebs.io/component-name": "ndm-config",
			},
		},
		Data: map[string]string{
			"node-disk-manager.config": string(data),
		},
	}, nil
}

type NDMProbe struct {
	Name  string `yaml:"name"`
	Key   string `yaml:"key"`
	State bool   `yaml:"state"`
}

type NDMFilter struct {
	Name    string `yaml:"name"`
	Key     string `yaml:"key"`
	State   bool   `yaml:"state"`
	Include string `yaml:"include,omitempty"`
	Exclude string `yaml:"exclude,omitempty"`
}

var (
	defaultMountExcludes  = []string{"/", "/etc/hosts", "/boot"}
	defaultVendorExcludes = []string{"CLOUDBYT", "OpenEBS"}
	defaultDeviceExcludes = []string{"loop", "/dev/fd0", "/dev/sr0", "/dev/ram", "/dev/dm-", "/dev/md"}
)
