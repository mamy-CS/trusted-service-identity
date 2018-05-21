/*
Copyright 2018 The Kubernetes Authors.

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

// Code generated by informer-gen. DO NOT EDIT.

// This file was automatically generated by informer-gen

package v1

import (
	time "time"

	pti_v1 "github.ibm.com/brandon-lum/ti-keyrelease/pkg/apis/pti/v1"
	versioned "github.ibm.com/brandon-lum/ti-keyrelease/pkg/client/clientset/versioned"
	internalinterfaces "github.ibm.com/brandon-lum/ti-keyrelease/pkg/client/informers/externalversions/internalinterfaces"
	v1 "github.ibm.com/brandon-lum/ti-keyrelease/pkg/client/listers/pti/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// PodTIInformer provides access to a shared informer and lister for
// PodTIs.
type PodTIInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1.PodTILister
}

type podTIInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewPodTIInformer constructs a new informer for PodTI type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewPodTIInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredPodTIInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredPodTIInformer constructs a new informer for PodTI type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredPodTIInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.PtiV1().PodTIs(namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.PtiV1().PodTIs(namespace).Watch(options)
			},
		},
		&pti_v1.PodTI{},
		resyncPeriod,
		indexers,
	)
}

func (f *podTIInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredPodTIInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *podTIInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&pti_v1.PodTI{}, f.defaultInformer)
}

func (f *podTIInformer) Lister() v1.PodTILister {
	return v1.NewPodTILister(f.Informer().GetIndexer())
}
