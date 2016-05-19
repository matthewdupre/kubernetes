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

package etcd

import (
	"testing"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/registry/generic"
	"k8s.io/kubernetes/pkg/registry/registrytest"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/storage/etcd/etcdtest"
	etcdtesting "k8s.io/kubernetes/pkg/storage/etcd/testing"
	"k8s.io/kubernetes/pkg/util/diff"
)

const defaultReplicas = 100

func newStorage(t *testing.T) (*NetworkPolicyStorage, *etcdtesting.EtcdTestServer) {
	etcdStorage, server := registrytest.NewEtcdStorage(t, "extensions")
	restOptions := generic.RESTOptions{Storage: etcdStorage, Decorator: generic.UndecoratedStorage, DeleteCollectionWorkers: 1}
	networkPolicyStorage := NewStorage(restOptions)
	return &networkPolicyStorage, server
}

// createNetworkPolicy is a helper function that returns a NetworkPolicy with the updated resource version.
func createNetworkPolicy(storage *REST, np extensions.NetworkPolicy, t *testing.T) (extensions.NetworkPolicy, error) {
	ctx := api.WithNamespace(api.NewContext(), np.Namespace)
	obj, err := storage.Create(ctx, &np)
	if err != nil {
		t.Errorf("Failed to create NetworkPolicy, %v", err)
	}
	newNP := obj.(*extensions.NetworkPolicy)
	return *newNP, nil
}

func validNewNetworkPolicy() *extensions.NetworkPolicy {
	return &extensions.NetworkPolicy{
		ObjectMeta: api.ObjectMeta{
			Name:      "foo",
			Namespace: api.NamespaceDefault,
		},
		Spec: extensions.NetworkPolicySpec{
			Selector: &unversioned.LabelSelector{MatchLabels: map[string]string{"a": "b"}},
			Template: api.PodTemplateSpec{
				ObjectMeta: api.ObjectMeta{
					Labels: map[string]string{"a": "b"},
				},
				Spec: api.PodSpec{
					Containers: []api.Container{
						{
							Name:            "test",
							Image:           "test_image",
							ImagePullPolicy: api.PullIfNotPresent,
						},
					},
					RestartPolicy: api.RestartPolicyAlways,
					DNSPolicy:     api.DNSClusterFirst,
				},
			},
			Replicas: 7,
		},
		Status: extensions.NetworkPolicyStatus{
			Replicas: 5,
		},
	}
}

var validNetworkPolicy = *validNewNetworkPolicy()

func TestCreate(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	test := registrytest.New(t, storage.NetworkPolicy.Store)
	np := validNewNetworkPolicy()
	np.ObjectMeta = api.ObjectMeta{}
	test.TestCreate(
		// valid
		np,
		// invalid (invalid selector)
		&extensions.NetworkPolicy{
			Spec: extensions.NetworkPolicySpec{
				Replicas: 2,
				Selector: &unversioned.LabelSelector{MatchLabels: map[string]string{}},
				Template: validNetworkPolicy.Spec.Template,
			},
		},
	)
}

func TestUpdate(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	test := registrytest.New(t, storage.NetworkPolicy.Store)
	test.TestUpdate(
		// valid
		validNewNetworkPolicy(),
		// valid updateFunc
		func(obj runtime.Object) runtime.Object {
			object := obj.(*extensions.NetworkPolicy)
			object.Spec.Replicas = object.Spec.Replicas + 1
			return object
		},
		// invalid updateFunc
		func(obj runtime.Object) runtime.Object {
			object := obj.(*extensions.NetworkPolicy)
			object.Name = ""
			return object
		},
		func(obj runtime.Object) runtime.Object {
			object := obj.(*extensions.NetworkPolicy)
			object.Spec.Selector = &unversioned.LabelSelector{MatchLabels: map[string]string{}}
			return object
		},
	)
}

func TestDelete(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	test := registrytest.New(t, storage.NetworkPolicy.Store)
	test.TestDelete(validNewNetworkPolicy())
}

func TestGenerationNumber(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	modifiedSno := *validNewNetworkPolicy()
	modifiedSno.Generation = 100
	modifiedSno.Status.ObservedGeneration = 10
	ctx := api.NewDefaultContext()
	np, err := createNetworkPolicy(storage.NetworkPolicy, modifiedSno, t)
	etcdNP, err := storage.NetworkPolicy.Get(ctx, np.Name)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	storedNP, _ := etcdNP.(*extensions.NetworkPolicy)

	// Generation initialization
	if storedNP.Generation != 1 && storedNP.Status.ObservedGeneration != 0 {
		t.Fatalf("Unexpected generation number %v, status generation %v", storedNP.Generation, storedNP.Status.ObservedGeneration)
	}

	// Updates to spec should increment the generation number
	storedNP.Spec.Replicas += 1
	storage.NetworkPolicy.Update(ctx, storedNP)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	etcdNP, err = storage.NetworkPolicy.Get(ctx, np.Name)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	storedNP, _ = etcdNP.(*extensions.NetworkPolicy)
	if storedNP.Generation != 2 || storedNP.Status.ObservedGeneration != 0 {
		t.Fatalf("Unexpected generation, spec: %v, status: %v", storedNP.Generation, storedNP.Status.ObservedGeneration)
	}

	// Updates to status should not increment either spec or status generation numbers
	storedNP.Status.Replicas += 1
	storage.NetworkPolicy.Update(ctx, storedNP)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	etcdNP, err = storage.NetworkPolicy.Get(ctx, np.Name)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	storedNP, _ = etcdNP.(*extensions.NetworkPolicy)
	if storedNP.Generation != 2 || storedNP.Status.ObservedGeneration != 0 {
		t.Fatalf("Unexpected generation number, spec: %v, status: %v", storedNP.Generation, storedNP.Status.ObservedGeneration)
	}
}

func TestGet(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	test := registrytest.New(t, storage.NetworkPolicy.Store)
	test.TestGet(validNewNetworkPolicy())
}

func TestList(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	test := registrytest.New(t, storage.NetworkPolicy.Store)
	test.TestList(validNewNetworkPolicy())
}

func TestWatch(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	test := registrytest.New(t, storage.NetworkPolicy.Store)
	test.TestWatch(
		validNewNetworkPolicy(),
		// matching labels
		[]labels.Set{
			{"a": "b"},
		},
		// not matching labels
		[]labels.Set{
			{"a": "c"},
			{"foo": "bar"},
		},
		// matching fields
		[]fields.Set{
			{"status.replicas": "5"},
			{"metadata.name": "foo"},
			{"status.replicas": "5", "metadata.name": "foo"},
		},
		// not matchin fields
		[]fields.Set{
			{"status.replicas": "10"},
			{"metadata.name": "bar"},
			{"name": "foo"},
			{"status.replicas": "10", "metadata.name": "foo"},
			{"status.replicas": "0", "metadata.name": "bar"},
		},
	)
}

func TestScaleGet(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)

	name := "foo"

	var np extensions.NetworkPolicy
	ctx := api.WithNamespace(api.NewContext(), api.NamespaceDefault)
	key := etcdtest.AddPrefix("/replicasets/" + api.NamespaceDefault + "/" + name)
	if err := storage.NetworkPolicy.Storage.Create(ctx, key, &validNetworkPolicy, &np, 0); err != nil {
		t.Fatalf("error setting new network policy (key: %s) %v: %v", key, validNetworkPolicy, err)
	}

	want := &extensions.Scale{
		ObjectMeta: api.ObjectMeta{
			Name:              name,
			Namespace:         api.NamespaceDefault,
			UID:               np.UID,
			ResourceVersion:   np.ResourceVersion,
			CreationTimestamp: np.CreationTimestamp,
		},
		Spec: extensions.ScaleSpec{
			Replicas: validNetworkPolicy.Spec.Replicas,
		},
		Status: extensions.ScaleStatus{
			Replicas: validNetworkPolicy.Status.Replicas,
			Selector: validNetworkPolicy.Spec.Selector,
		},
	}
	obj, err := storage.Scale.Get(ctx, name)
	got := obj.(*extensions.Scale)
	if err != nil {
		t.Fatalf("error fetching scale for %s: %v", name, err)
	}
	if !api.Semantic.DeepEqual(got, want) {
		t.Errorf("unexpected scale: %s", diff.ObjectDiff(got, want))
	}
}

func TestScaleUpdate(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)

	name := "foo"

	var np extensions.NetworkPolicy
	ctx := api.WithNamespace(api.NewContext(), api.NamespaceDefault)
	key := etcdtest.AddPrefix("/networkpolicies/" + api.NamespaceDefault + "/" + name)
	if err := storage.NetworkPolicy.Storage.Create(ctx, key, &validNetworkPolicy, &np, 0); err != nil {
		t.Fatalf("error setting new network policy (key: %s) %v: %v", key, validNetworkPolicy, err)
	}
	replicas := 12
	update := extensions.Scale{
		ObjectMeta: api.ObjectMeta{
			Name:      name,
			Namespace: api.NamespaceDefault,
		},
		Spec: extensions.ScaleSpec{
			Replicas: int32(replicas),
		},
	}

	if _, _, err := storage.Scale.Update(ctx, &update); err != nil {
		t.Fatalf("error updating scale %v: %v", update, err)
	}

	obj, err := storage.Scale.Get(ctx, name)
	if err != nil {
		t.Fatalf("error fetching scale for %s: %v", name, err)
	}
	scale := obj.(*extensions.Scale)
	if scale.Spec.Replicas != int32(replicas) {
		t.Errorf("wrong replicas count expected: %d got: %d", replicas, scale.Spec.Replicas)
	}

	update.ResourceVersion = np.ResourceVersion
	update.Spec.Replicas = 15

	if _, _, err = storage.Scale.Update(ctx, &update); err != nil && !errors.IsConflict(err) {
		t.Fatalf("unexpected error, expecting an update conflict but got %v", err)
	}
}

func TestStatusUpdate(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)

	ctx := api.WithNamespace(api.NewContext(), api.NamespaceDefault)
	key := etcdtest.AddPrefix("/networkpolicies/" + api.NamespaceDefault + "/foo")
	if err := storage.NetworkPolicy.Storage.Create(ctx, key, &validNetworkPolicy, nil, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	update := extensions.NetworkPolicy{
		ObjectMeta: validNetworkPolicy.ObjectMeta,
		Spec: extensions.NetworkPolicySpec{
			Replicas: defaultReplicas,
		},
		Status: extensions.NetworkPolicyStatus{
			Replicas: defaultReplicas,
		},
	}

	if _, _, err := storage.Status.Update(ctx, &update); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	obj, err := storage.NetworkPolicy.Get(ctx, "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	np := obj.(*extensions.NetworkPolicy)
	if np.Spec.Replicas != 7 {
		t.Errorf("we expected .spec.replicas to not be updated but it was updated to %v", np.Spec.Replicas)
	}
	if np.Status.Replicas != defaultReplicas {
		t.Errorf("we expected .status.replicas to be updated to %d but it was %v", defaultReplicas, np.Status.Replicas)
	}
}
