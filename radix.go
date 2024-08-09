package radix

import (
	"runtime"
	"sort"
	"strings"
	"unsafe"
)

// WalkFn is used when walking the tree. Takes a
// key and value, returning if iteration should
// be terminated.
type WalkFn func(key string, node *Node) bool

// leafNode is used to represent a value
type leafNode struct {
	val interface{}
}

// edge is used to represent an edge Node
type edge struct {
	label byte
	node  *Node
}

type Node struct {
	// leaf is used to store possible leaf
	leaf *leafNode

	// prefix is the common prefix we ignore
	prefix string

	// Edges should be stored in-order for iteration.
	// We avoid a fully materialized slice to save memory,
	// since in most cases we expect to be sparse
	edges edges
}

func (n *Node) GetPrefix() string {
	return n.prefix
}

func (n *Node) GetValue() interface{} {
	if n.leaf == nil {
		return ""
	}
	return n.leaf.val
}

func (n *Node) isLeaf() bool {
	return n.leaf != nil
}

func (n *Node) addEdge(e edge) {
	num := len(n.edges)
	idx := sort.Search(num, func(i int) bool {
		return n.edges[i].label >= e.label
	})

	n.edges = append(n.edges, edge{})
	copy(n.edges[idx+1:], n.edges[idx:])
	n.edges[idx] = e
}

func (n *Node) updateEdge(label byte, node *Node) {
	num := len(n.edges)
	idx := sort.Search(num, func(i int) bool {
		return n.edges[i].label >= label
	})
	if idx < num && n.edges[idx].label == label {
		n.edges[idx].node = node
		return
	}
	panic("replacing missing edge")
}

func (n *Node) getEdge(label byte) *Node {
	num := len(n.edges)
	idx := sort.Search(num, func(i int) bool {
		return n.edges[i].label >= label
	})
	if idx < num && n.edges[idx].label == label {
		return n.edges[idx].node
	}
	return nil
}

func (n *Node) delEdge(label byte) {
	num := len(n.edges)
	idx := sort.Search(num, func(i int) bool {
		return n.edges[i].label >= label
	})
	if idx < num && n.edges[idx].label == label {
		copy(n.edges[idx:], n.edges[idx+1:])
		n.edges[len(n.edges)-1] = edge{}
		n.edges = n.edges[:len(n.edges)-1]
	}
}

type edges []edge

func (e edges) Len() int {
	return len(e)
}

func (e edges) Less(i, j int) bool {
	return e[i].label < e[j].label
}

func (e edges) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

func (e edges) Sort() {
	sort.Sort(e)
}

// Tree implements a radix tree. This can be treated as a
// Dictionary abstract data type. The main advantage over
// a standard hash map is prefix-based lookups and
// ordered iteration,
type Tree struct {
	root            *Node
	size            int
	memorySize      *uintptr // only post func Optimize()
	optimizeEnabled bool
}

// New returns an empty Tree
func New() *Tree {
	return NewFromMap(nil)
}

// NewFromMap returns a new tree containing the keys
// from an existing map
func NewFromMap(m map[string]interface{}) *Tree {
	t := &Tree{root: &Node{}}
	for k, v := range m {
		t.Insert(k, v)
	}
	return t
}

// Len is used to return the number of elements in the tree
func (t *Tree) Len() int {
	return t.size
}

func (t *Tree) MemoryLen() *uintptr {
	return t.memorySize
}

// longestPrefix finds the length of the shared prefix
// of two strings
func longestPrefix(k1, k2 string) int {
	max := len(k1)
	if l := len(k2); l < max {
		max = l
	}
	var i int
	for i = 0; i < max; i++ {
		if k1[i] != k2[i] {
			break
		}
	}
	return i
}

func (t *Tree) Optimize(runGC bool) {
	// clear capacity
	t.optimizeEnabled = true
	defer func() {
		if runGC {
			runtime.GC()
		}
		t.optimizeEnabled = false
	}()
	v := unsafe.Sizeof(t)
	if t.root != nil && len(t.root.edges) > 0 {
		v += unsafe.Sizeof(t.root)
		t.root.edges = t.root.edges[:len(t.root.edges):len(t.root.edges)]
		v += unsafe.Sizeof(t.root.edges)
	}

	t.Walk(func(key string, node *Node) bool {
		v += unsafe.Sizeof(node)
		if len(node.edges) > 0 {
			node.edges = node.edges[:len(node.edges):len(node.edges)]
			v += unsafe.Sizeof(node.edges)
		}
		return false
	})
	t.memorySize = &v
}

func (t *Tree) Insert(s string, v interface{}) (interface{}, bool) {
	var parent *Node
	n := t.root
	search := s
	for {
		// Handle key exhaution
		if len(search) == 0 {
			if n.isLeaf() {
				old := n.leaf.val
				n.leaf.val = v
				return old, true
			}

			n.leaf = &leafNode{
				val: v,
			}
			t.size++
			return nil, false
		}

		// Look for the edge
		parent = n
		n = n.getEdge(search[0])

		// No edge, create one
		if n == nil {
			e := edge{
				label: search[0],
				node: &Node{
					leaf: &leafNode{
						val: v,
					},
					prefix: search,
				},
			}
			parent.addEdge(e)
			t.size++
			return nil, false
		}

		// Determine longest prefix of the search key on match
		commonPrefix := longestPrefix(search, n.prefix)
		if commonPrefix == len(n.prefix) {
			search = search[commonPrefix:]
			continue
		}

		// Split the Node
		t.size++
		child := &Node{
			prefix: search[:commonPrefix],
		}
		parent.updateEdge(search[0], child)

		// Restore the existing Node
		child.addEdge(edge{
			label: n.prefix[commonPrefix],
			node:  n,
		})
		n.prefix = n.prefix[commonPrefix:]

		// Create a new leaf Node
		leaf := &leafNode{
			val: v,
		}

		// If the new key is a subset, add to this Node
		search = search[commonPrefix:]
		if len(search) == 0 {
			child.leaf = leaf
			return nil, false
		}

		// Create a new edge for the Node
		child.addEdge(edge{
			label: search[0],
			node: &Node{
				leaf:   leaf,
				prefix: search,
			},
		})
		return nil, false
	}
}

// Delete is used to delete a key, returning the previous
// value and if it was deleted
func (t *Tree) Delete(s string) (interface{}, bool) {
	var parent *Node
	var label byte
	n := t.root
	search := s
	for {
		// Check for key exhaution
		if len(search) == 0 {
			if !n.isLeaf() {
				break
			}
			goto DELETE
		}

		// Look for an edge
		parent = n
		label = search[0]
		n = n.getEdge(label)
		if n == nil {
			break
		}

		// Consume the search prefix
		if strings.HasPrefix(search, n.prefix) {
			search = search[len(n.prefix):]
		} else {
			break
		}
	}
	return nil, false

DELETE:
	// Delete the leaf
	leaf := n.leaf
	n.leaf = nil
	t.size--

	// Check if we should delete this Node from the parent
	if parent != nil && len(n.edges) == 0 {
		parent.delEdge(label)
	}

	// Check if we should merge this Node
	if n != t.root && len(n.edges) == 1 {
		n.mergeChild()
	}

	// Check if we should merge the parent's other child
	if parent != nil && parent != t.root && len(parent.edges) == 1 && !parent.isLeaf() {
		parent.mergeChild()
	}

	return leaf.val, true
}

// DeletePrefix is used to delete the subtree under a prefix
// Returns how many nodes were deleted
// Use this to delete large subtrees efficiently
func (t *Tree) DeletePrefix(s string) int {
	return t.deletePrefix(nil, t.root, s)
}

// delete does a recursive deletion
func (t *Tree) deletePrefix(parent, n *Node, prefix string) int {
	// Check for key exhaustion
	if len(prefix) == 0 {
		// Remove the leaf Node
		subTreeSize := 0
		//recursively walk from all edges of the Node to be deleted
		t.recursiveWalk(n, func(s string, node *Node) bool {
			subTreeSize++
			return false
		}, prefix)
		if n.isLeaf() {
			n.leaf = nil
		}
		n.edges = nil // deletes the entire subtree

		// Check if we should merge the parent's other child
		if parent != nil && parent != t.root && len(parent.edges) == 1 && !parent.isLeaf() {
			parent.mergeChild()
		}
		t.size -= subTreeSize
		return subTreeSize
	}

	// Look for an edge
	label := prefix[0]
	child := n.getEdge(label)
	if child == nil || (!strings.HasPrefix(child.prefix, prefix) && !strings.HasPrefix(prefix, child.prefix)) {
		return 0
	}

	// Consume the search prefix
	if len(child.prefix) > len(prefix) {
		prefix = prefix[len(prefix):]
	} else {
		prefix = prefix[len(child.prefix):]
	}
	return t.deletePrefix(n, child, prefix)
}

func (n *Node) mergeChild() {
	e := n.edges[0]
	child := e.node
	n.prefix = n.prefix + child.prefix
	n.leaf = child.leaf
	n.edges = child.edges
}

// Get is used to lookup a specific key, returning
// the value and if it was found
func (t *Tree) Get(s string) (interface{}, bool) {
	n := t.root
	search := s
	for {
		// Check for key exhaution
		if len(search) == 0 {
			if n.isLeaf() {
				return n.leaf.val, true
			}
			break
		}

		// Look for an edge
		n = n.getEdge(search[0])
		if n == nil {
			break
		}

		// Consume the search prefix
		if strings.HasPrefix(search, n.prefix) {
			search = search[len(n.prefix):]
		} else {
			break
		}
	}
	return nil, false
}

// Get is used to lookup a specific key, returning
// the value and if it was found
func (t *Tree) GetLastEqual(s string) (interface{}, bool) {
	n := t.root
	search := s
	var prevM *Node
	for {
		// Check for key exhaution
		if len(search) == 0 {
			if n.isLeaf() {
				return n.leaf.val, true
			}
			break
		}

		// Look for an edge
		n = n.getEdge(search[0])
		if n == nil {
			if prevM != n && prevM.leaf != nil {
				return prevM.leaf.val, true
			}
			break
		}
		prevM = n
		// Consume the search prefix
		if strings.HasPrefix(search, n.prefix) {
			search = search[len(n.prefix):]
		} else {
			break
		}
	}
	return nil, false
}

// LongestPrefix is like Get, but instead of an
// exact match, it will return the longest prefix match.
func (t *Tree) LongestPrefix(s string) (string, interface{}, bool) {
	var last *leafNode
	n := t.root
	search := s
	key := ""
	for {
		// Look for a leaf Node
		if n.isLeaf() {
			last = n.leaf
		}

		// Check for key exhaution
		if len(search) == 0 {
			break
		}

		// Look for an edge
		n = n.getEdge(search[0])
		if n == nil {
			break
		}

		// Consume the search prefix
		if strings.HasPrefix(search, n.prefix) {
			search = search[len(n.prefix):]
			key += n.prefix
		} else {
			break
		}
	}
	if last != nil {
		return key, last.val, true
	}
	return "", nil, false
}

// Minimum is used to return the minimum value in the tree
func (t *Tree) Minimum() (string, interface{}, bool) {
	n := t.root
	key := ""
	for {
		if n.isLeaf() {
			return key, n.leaf.val, true
		}
		if len(n.edges) > 0 {
			n = n.edges[0].node
			key += n.prefix
		} else {
			break
		}
	}
	return key, nil, false
}

// Maximum is used to return the maximum value in the tree
func (t *Tree) Maximum() (string, interface{}, bool) {
	n := t.root
	key := ""
	for {
		if num := len(n.edges); num > 0 {
			n = n.edges[num-1].node
			key += n.prefix
			continue
		}
		if n.isLeaf() {
			return key, n.leaf.val, true
		}
		break
	}
	return key, nil, false
}

// Walk is used to walk the tree
func (t *Tree) Walk(fn WalkFn) {
	t.recursiveWalk(t.root, fn, "")
}

// WalkPrefix is used to walk the tree under a prefix
func (t *Tree) WalkPrefix(prefix string, fn WalkFn) {
	n := t.root
	search := prefix
	key := ""
	for {
		// Check for key exhaustion
		if len(search) == 0 {
			t.recursiveWalk(n, fn, key)
			return
		}

		// Look for an edge
		n = n.getEdge(search[0])
		if n == nil {
			return
		}

		// Consume the search prefix
		if strings.HasPrefix(search, n.prefix) {
			search = search[len(n.prefix):]
			key += n.prefix
			continue
		}
		if strings.HasPrefix(n.prefix, search) {
			// Child may be under our search prefix
			key = key + n.prefix
			t.recursiveWalk(n, fn, key)
		}
		return
	}
}

// WalkPath is used to walk the tree, but only visiting nodes
// from the root down to a given leaf. Where WalkPrefix walks
// all the entries *under* the given prefix, this walks the
// entries *above* the given prefix.
func (t *Tree) WalkPath(path string, fn WalkFn) {
	n := t.root
	search := path
	key := ""
	for {
		// Visit the leaf values if any
		if n.leaf != nil && fn(key, n) {
			return
		}

		// Check for key exhaution
		if len(search) == 0 {
			return
		}

		// Look for an edge
		n = n.getEdge(search[0])
		if n == nil {
			return
		}

		// Consume the search prefix
		if strings.HasPrefix(search, n.prefix) {
			search = search[len(n.prefix):]
			key += n.prefix
		} else {
			break
		}
	}
}

// recursiveWalk is used to do a pre-order walk of a Node
// recursively. Returns true if the walk should be aborted
func (t *Tree) recursiveWalk(n *Node, fn WalkFn, key string) bool {
	return t._recursiveWalk(n, fn, key)
}

// recursiveWalk is used to do a pre-order walk of a Node
// recursively. Returns true if the walk should be aborted
func (t *Tree) _recursiveWalk(n *Node, fn WalkFn, key string) bool {
	// Visit the leaf values if any
	if len(n.edges) > 0 && t.optimizeEnabled {
		fn(key, n)
	}

	if n.leaf != nil && fn(key, n) {
		return true
	}

	// Recurse on the children
	i := 0
	k := len(n.edges) // keeps track of number of edges in previous iteration
	for i < k {
		e := n.edges[i]
		if t._recursiveWalk(e.node, fn, key+e.node.prefix) {
			return true
		}
		// It is a possibility that the WalkFn modified the Node we are
		// iterating on. If there are no more edges, mergeChild happened,
		// so the last edge became the current Node n, on which we'll
		// iterate one last time.
		if len(n.edges) == 0 {
			keyRune := []rune(key)
			keyNodeRune := []rune(n.prefix)
			needKey := ""
			for i := range keyRune {
				if keyRune[i] != keyNodeRune[0] {
					needKey += string(keyRune[i])
				} else {
					break
				}
			}
			needKey += n.prefix
			return t._recursiveWalk(n, fn, needKey)
		}
		// If there are now less edges than in the previous iteration,
		// then do not increment the loop index, since the current index
		// points to a new edge. Otherwise, get to the next index.
		if len(n.edges) >= k {
			i++
		}
		k = len(n.edges)
	}
	return false
}

// ToMap is used to walk the tree and convert it into a map
func (t *Tree) ToMap() map[string]interface{} {
	out := make(map[string]interface{}, t.size)
	t.Walk(func(k string, node *Node) bool {
		out[k] = node.leaf.val
		return false
	})
	return out
}
