/*
Copyright © 2023 The Authors (See AUTHORS file)

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
package cache

var (
	defaultStore Store = NewHashMapStore() // Default underlying cache implementation
	allGroups          = map[string]bool{} // To avoid instanciating the same group twice
)

// Sets the default store for all the next groups to be created
func SetDefaultStore(store Store) {
	defaultStore = store
}

type Group[K comparable, V any] struct {
	store Store // The underlying cache engine
	name  string
	load  func(key K) (V, error)

	// loadGroup ensures that each key is only fetched once
	// (either locally or remotely), regardless of the number of
	// concurrent callers.
	loadGroup flightGroup[K, V]

	// messageBroker is used for clustered events like flushing of entries
	messageBroker MessageBroker
}

// flightGroup is defined as an interface which flightgroup.Group
// satisfies.  We define this so that we may test with an alternate
// implementation.
type flightGroup[K comparable, V any] interface {
	Do(key K, fn func() (V, error)) (V, error)
}

func (g *Group[K, V]) Get(key K) (V, error) {
	gk := g.store.Key(g.name, key)
	if v, ok := g.store.Get(gk); ok {
		return v.(V), nil
	}

	loadAndSet := func() (V, error) {
		// Not found in cache, using loader
		v, err := g.load(key)

		if err != nil {
			return v, err
		}

		// Set the value
		g.store.Set(gk, v)
		return v, nil
	}

	if g.loadGroup != nil {
		return g.loadGroup.Do(key, loadAndSet)
	}
	return loadAndSet()
}

func (g *Group[K, V]) Del(key K) {
	gk := g.store.Key(g.name, key)
	g.store.Del(gk)
	if g.messageBroker != nil {
		msg := cacheMsg{Group: g.name, Key: key}
		g.messageBroker.Send(msg.bytes())
	}
}

func (g *Group[K, V]) Clear() {
	g.store.Clear(g.name)
	if g.messageBroker != nil {
		msg := cacheMsg{Group: g.name, Key: nil}
		g.messageBroker.Send(msg.bytes())
	}
}
