/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package networkpolicy

import (
	"testing"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/extensions"
)

func TestNetworkPolicyStrategy(t *testing.T) {
	ctx := api.NewDefaultContext()
	if !Strategy.NamespaceScoped() {
		t.Errorf("NetworkPolicy must be namespace scoped")
	}
	if Strategy.AllowCreateOnUpdate() {
		t.Errorf("NetworkPolicy should not allow create on update")
	}

	validSelector := map[string]string{"a": "b"}
	validPodTemplate := api.PodTemplate{
		Template: api.PodTemplateSpec{
			ObjectMeta: api.ObjectMeta{
				Labels: validSelector,
			},
			Spec: api.PodSpec{
				RestartPolicy: api.RestartPolicyAlways,
				DNSPolicy:     api.DNSClusterFirst,
				Containers:    []api.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent"}},
			},
		},
	}
	rs := &extensions.NetworkPolicy{
		ObjectMeta: api.ObjectMeta{Name: "abc", Namespace: api.NamespaceDefault},
		Spec: extensions.NetworkPolicySpec{
			Selector: &unversioned.LabelSelector{MatchLabels: validSelector},
			Template: validPodTemplate.Template,
		},
		Status: extensions.NetworkPolicyStatus{
			Replicas:           1,
			ObservedGeneration: int64(10),
		},
	}

	Strategy.PrepareForCreate(rs)
	if rs.Status.Replicas != 0 {
		t.Error("NetworkPolicy should not allow setting status.replicas on create")
	}
	if rs.Status.ObservedGeneration != int64(0) {
		t.Error("NetworkPolicy should not allow setting status.observedGeneration on create")
	}
	errs := Strategy.Validate(ctx, rs)
	if len(errs) != 0 {
		t.Errorf("Unexpected error validating %v", errs)
	}

	invalidRc := &extensions.NetworkPolicy{
		ObjectMeta: api.ObjectMeta{Name: "bar", ResourceVersion: "4"},
	}
	Strategy.PrepareForUpdate(invalidRc, rs)
	errs = Strategy.ValidateUpdate(ctx, invalidRc, rs)
	if len(errs) == 0 {
		t.Errorf("Expected a validation error")
	}
	if invalidRc.ResourceVersion != "4" {
		t.Errorf("Incoming resource version on update should not be mutated")
	}
}

func TestNetworkPolicyStatusStrategy(t *testing.T) {
	ctx := api.NewDefaultContext()
	if !StatusStrategy.NamespaceScoped() {
		t.Errorf("NetworkPolicy must be namespace scoped")
	}
	if StatusStrategy.AllowCreateOnUpdate() {
		t.Errorf("NetworkPolicy should not allow create on update")
	}
	validSelector := map[string]string{"a": "b"}
	validPodTemplate := api.PodTemplate{
		Template: api.PodTemplateSpec{
			ObjectMeta: api.ObjectMeta{
				Labels: validSelector,
			},
			Spec: api.PodSpec{
				RestartPolicy: api.RestartPolicyAlways,
				DNSPolicy:     api.DNSClusterFirst,
				Containers:    []api.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent"}},
			},
		},
	}
	oldNP := &extensions.NetworkPolicy{
		ObjectMeta: api.ObjectMeta{Name: "abc", Namespace: api.NamespaceDefault, ResourceVersion: "10"},
		Spec: extensions.NetworkPolicySpec{
			Replicas: 3,
			Selector: &unversioned.LabelSelector{MatchLabels: validSelector},
			Template: validPodTemplate.Template,
		},
		Status: extensions.NetworkPolicyStatus{
			Replicas:           1,
			ObservedGeneration: int64(10),
		},
	}
	newNP := &extensions.NetworkPolicy{
		ObjectMeta: api.ObjectMeta{Name: "abc", Namespace: api.NamespaceDefault, ResourceVersion: "9"},
		Spec: extensions.NetworkPolicySpec{
			Replicas: 1,
			Selector: &unversioned.LabelSelector{MatchLabels: validSelector},
			Template: validPodTemplate.Template,
		},
		Status: extensions.NetworkPolicyStatus{
			Replicas:           3,
			ObservedGeneration: int64(11),
		},
	}
	StatusStrategy.PrepareForUpdate(newNP, oldNP)
	if newNP.Status.Replicas != 3 {
		t.Errorf("NetworkPolicy status updates should allow change of replicas: %v", newNP.Status.Replicas)
	}
	if newNP.Spec.Replicas != 3 {
		t.Errorf("PrepareForUpdate should have preferred spec")
	}
	errs := StatusStrategy.ValidateUpdate(ctx, newNP, oldNP)
	if len(errs) != 0 {
		t.Errorf("Unexpected error %v", errs)
	}
}
