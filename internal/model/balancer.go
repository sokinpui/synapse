package model

import "sync"

// api key table
// index | key | used
type apiKeyState struct {
	Value string
	Used  bool
}

type KeyBalancer struct {
	keys []apiKeyState
	mu   sync.Mutex
}

func NewKeyBalancer(apiKeys []string) *KeyBalancer {
	states := make([]apiKeyState, len(apiKeys))
	for i, key := range apiKeys {
		states[i] = apiKeyState{Value: key, Used: false}
	}
	return &KeyBalancer{keys: states}
}

func (b *KeyBalancer) PickKey() (string, int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.keys) == 0 {
		return "", -1
	}

	for i := range b.keys {
		if b.keys[i].Used {
			continue
		}

		key := b.keys[i].Value
		b.keys[i].Used = true

		if b.areAllUsed() {
			b.reset()
		}

		return key, i
	}

	b.reset()
	b.keys[0].Used = true
	return b.keys[0].Value, 0
}

func (b *KeyBalancer) KeyCount() int {
	return len(b.keys)
}

func (b *KeyBalancer) areAllUsed() bool {
	for _, k := range b.keys {
		if !k.Used {
			return false
		}
	}
	return true
}

func (b *KeyBalancer) reset() {
	for i := range b.keys {
		b.keys[i].Used = false
	}
}
