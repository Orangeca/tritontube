package chash

import (
	"crypto/sha1"
	"encoding/binary"
	"sort"
	"strconv"
)

// 虚拟节点 token
type token struct {
	hash uint64
	node string // 节点标识（这里直接用 baseURL 或 nodeID）
}

type Ring struct {
	vnodes int     // 每个真实节点对应的虚拟节点数
	tokens []token // 排序后的环
}

// vnodes: 每个真实节点的虚拟节点数（建议 100~200 起步）
func NewRing(vnodes int) *Ring {
	if vnodes <= 0 {
		vnodes = 100
	}
	return &Ring{vnodes: vnodes}
}

func (r *Ring) hash(b []byte) uint64 {
	h := sha1.Sum(b)
	return binary.BigEndian.Uint64(h[:8]) // 取前 8 字节够用了
}

// AddNode 将一个真实节点映射为多个虚拟节点挂到环上
func (r *Ring) AddNode(nodeID string) {
	for i := 0; i < r.vnodes; i++ {
		h := r.hash([]byte(nodeID + "#" + strconv.Itoa(i)))
		r.tokens = append(r.tokens, token{hash: h, node: nodeID})
	}
	sort.Slice(r.tokens, func(i, j int) bool { return r.tokens[i].hash < r.tokens[j].hash })
}

func (r *Ring) RemoveNode(nodeID string) {
	out := r.tokens[:0]
	for _, t := range r.tokens {
		if t.node != nodeID {
			out = append(out, t)
		}
	}
	r.tokens = out
	// r.tokens 已经保持近似有序（删除不破坏有序性）
}

// Lookup 返回按一致性哈希顺时针找的副本列表（去重，不超过 replicas）
func (r *Ring) Lookup(key []byte, replicas int) []string {
	if replicas <= 0 || len(r.tokens) == 0 {
		return nil
	}
	h := r.hash(key)
	// 找到第一个 >= h 的位置
	i := sort.Search(len(r.tokens), func(i int) bool { return r.tokens[i].hash >= h })
	if i == len(r.tokens) {
		i = 0
	}
	seen := make(map[string]struct{}, replicas)
	out := make([]string, 0, replicas)
	for k := 0; len(out) < replicas && k < len(r.tokens); k++ {
		n := r.tokens[(i+k)%len(r.tokens)].node
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}
