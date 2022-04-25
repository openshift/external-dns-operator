/*
Copyright 2021.

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

package test

import (
	"context"
	"testing"
	"time"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
)

// Event is a simplified representation of the watch event received from the controller runtime client.
type Event struct {
	EventType watch.EventType
	ObjType   string
	types.NamespacedName
}

// NewEvent returns an event instance created from the controller runtime's watch event.
func NewEvent(we watch.Event) Event {
	te := Event{
		EventType: we.Type,
	}
	switch obj := we.Object.(type) {
	case *corev1.Secret:
		te.ObjType = "secret"
		te.Namespace = obj.Namespace
		te.Name = obj.Name
	case *corev1.ConfigMap:
		te.ObjType = "configmap"
		te.Namespace = obj.Namespace
		te.Name = obj.Name
	case *appsv1.Deployment:
		te.ObjType = "deployment"
		te.Namespace = obj.Namespace
		te.Name = obj.Name
	case *corev1.ServiceAccount:
		te.ObjType = "serviceaccount"
		te.Namespace = obj.Namespace
		te.Name = obj.Name
	case *rbacv1.ClusterRole:
		te.ObjType = "clusterrole"
		te.Name = obj.Name
	case *rbacv1.ClusterRoleBinding:
		te.ObjType = "clusterrolebinding"
		te.Name = obj.Name
	case *corev1.Namespace:
		te.ObjType = "namespace"
		te.Name = obj.Name
	case *operatorv1beta1.ExternalDNS:
		te.ObjType = "externaldns"
		te.Name = obj.Name
	case *cco.CredentialsRequest:
		te.ObjType = "credentialsrequest"
		te.Name = obj.Name
	}
	return te
}

// Key returns a key like representation of the event.
func (e Event) Key() string {
	return string(e.EventType) + "/" + e.ObjType + "/" + e.Namespace + "/" + e.Name
}

// EventCollector collects all types of events for the given watch types.
type EventCollector struct {
	T          *testing.T
	Client     client.WithWatch
	WatchTypes []client.ObjectList
	Verbose    bool
	watches    []watch.Interface
	eventsCh   chan watch.Event
}

// NewEventCollector returns an instance of the event collector.
func NewEventCollector(t *testing.T, client client.WithWatch, watchTypes []client.ObjectList, bufSize int) *EventCollector {
	return &EventCollector{
		T:          t,
		Client:     client,
		WatchTypes: watchTypes,
		eventsCh:   make(chan watch.Event, bufSize),
	}
}

// Start starts watches for all the watch types.
func (c *EventCollector) Start(ctx context.Context) {
	c.T.Helper()

	for _, watchType := range c.WatchTypes {
		w, err := c.Client.Watch(ctx, watchType)
		if err != nil {
			c.T.Fatalf("failed to start the watch for %T: %v", watchType, err)
		}
		c.watches = append(c.watches, w)
	}

	// fan in the events
	for _, w := range c.watches {
		go func(ch <-chan watch.Event) {
			for e := range ch {
				if testing.Verbose() {
					c.T.Logf("Got watch event: %v", e)
				}
				c.eventsCh <- e
			}
		}(w.ResultChan())
	}
}

// Stop stops all the watches.
func (c *EventCollector) Stop() {
	for _, w := range c.watches {
		w.Stop()
	}
}

// Collect collects events until the given number is reached or until the timeout is expired.
func (c *EventCollector) Collect(num int, timeout time.Duration) []Event {
	res := []Event{}
out:
	for {
		select {
		case e := <-c.eventsCh:
			res = append(res, NewEvent(e))
			if len(res) == num {
				break out
			}
		case <-time.After(timeout):
			break out
		}
	}
	return res
}

// IndexEvents turns the slice of events into a map for the more convenient lookups.
func IndexEvents(events []Event) map[string]Event {
	m := map[string]Event{}
	for _, e := range events {
		m[e.Key()] = e
	}
	return m
}
