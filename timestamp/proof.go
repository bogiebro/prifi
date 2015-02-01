package timestamp

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"hash"
	"sort"

	"github.com/dedis/crypto/abstract"
)

type HashFunc func() hash.Hash

// Proof-of-beforeness:
// a list of offsets of peer-hash-pointers at each level below the root.

// Proof is used for Local Merkle Trees (computed based on messages from clients)
// One Proof sufficient for one leaf in a Local Merkle Tree
type Proof []HashId

// LevelProof is used for the Big Merkle Tree (computed from server commits)
// A []LevelProof from root to server is sufficient proof
type LevelProof []HashId

type hashContext struct {
	newHash func() hash.Hash
	hash    hash.Hash
}

func (c *hashContext) hashNode(buf []byte, vHashIds ...HashId) []byte {
	// unpack hashIds
	hashIds := make([]HashId, 0)
	for _, vHashId := range vHashIds {
		hashIds = append(hashIds, vHashId)
	}
	sort.Sort(ByHashId(hashIds))

	if c.hash == nil {
		c.hash = c.newHash()
	} else {
		c.hash.Reset()
	}
	h := c.hash

	for _, hashId := range hashIds {
		h.Write(hashId)

	}

	s := h.Sum(buf)
	return s
}

// Given a Proof and the hash of the leaf, compute the hash of the root.
// If the Proof is of length 0, simply returns leaf.
func (p Proof) Calc(newHash HashFunc, leaf []byte) []byte {
	c := hashContext{newHash: newHash}
	var buf []byte
	for i := len(p) - 1; i >= 0; i-- {
		leaf = c.hashNode(buf[:0], leaf, p[i])
		buf = leaf
	}
	return leaf
}

// Check a purported Proof against given root and leaf hashes.
func (p Proof) Check(newHash HashFunc, root, leaf []byte) bool {
	chk := p.Calc(newHash, leaf)
	// compare returns 1 if equal, so return is true when check is good
	// log.Println(chk, root)
	return subtle.ConstantTimeCompare(chk, root) != 0
}

func CheckProofs(newHash HashFunc, root HashId, leaf HashId,
	proofs Proof, levelProof LevelProof) bool {

	c := hashContext{newHash: newHash}
	// level proof has many leaves
	completeLevelProof := append(levelProof, leaf)
	leaf = c.hashNode(nil, completeLevelProof...)

	// checking with proof upwards, 2 hashids per hashnode
	for _, proof := range proofs {
		// level proof has the leaves for that level
		leaf = c.hashNode(nil, leaf, proof)
	}

	// last computed leaf should equal the root
	return subtle.ConstantTimeCompare(leaf, root) != 0
}

func CheckProof(newHash HashFunc, root HashId, leaf HashId, proof Proof) bool {
	// log.Println("Root", len(root), root)
	// log.Println("Leaf", len(leaf), leaf)
	// log.Println("Proof", proof)
	// log.Println("\n")
	if proof.Check(newHash, root, leaf) == false {
		panic("check failed at leaf")
	}
	return true
}

func CheckLocalProofs(newHash HashFunc, root HashId, leaves []HashId, proofs []Proof) bool {
	// fmt.Println("Created mtRoot:", mtRoot)

	for i := range proofs {
		// log.Println("Root", root)
		// log.Println("Leaf", leaves[i])
		// log.Println("Proof", proofs[i])
		// log.Println("\n")
		if proofs[i].Check(newHash, root, leaves[i]) == false {
			panic("check failed at leaf")
		}
	}
	return true
}

func (p *Proof) PrintProof(proofNumber int) {
	fmt.Println("Proof number=", proofNumber)
	for _, x := range *p {
		fmt.Println(x)
	}
	// 	fmt.Println("\n")
}

func PrintProofs(proofs []Proof) {
	for i, p := range proofs {
		p.PrintProof(i)
	}
}

func sibling(i int) int {
	if i&1 == 1 {
		return i - 1
	}
	return i + 1
}

// Generate a Merkle proof tree for the given list of leaves,
// yielding one output proof per leaf.
func ProofTree(newHash func() hash.Hash, leaves []HashId) (HashId, []Proof) {
	if len(leaves) == 0 {
		return HashId(""), nil
	}
	// Determine the required tree depth
	nleavesArg, nleaves := len(leaves), len(leaves)
	depth := 0
	for n := 1; n < nleaves; n <<= 1 {
		depth++
	}

	// if nleaves is not a power of 2, we add 0s to fill in up to pow2
	var i int
	for nleaves, i = (1 << uint(depth)), nleavesArg; i < nleaves; i++ {
		leaves = append(leaves, make([]byte, newHash().Size()))
	}
	// fmt.Println("depth=", depth, "nleaves=", nleavesArg)

	// Build the Merkle tree
	c := hashContext{newHash: newHash}
	tree := make([][]HashId, depth+1)
	tree[depth] = leaves
	nprev := nleaves
	tprev := tree[depth]
	for d := depth - 1; d >= 0; d-- {
		nnext := (nprev + 1) >> 1 // # hashes total at level i
		nnode := nprev >> 1       // # new nodes at level i
		// println("nprev", nprev, "nnext", nnext, "nnode", nnode)
		// fmt.Println("nprev", nprev, "nnext", nnext, "nnode", nnode)
		tree[d] = make([]HashId, nnext)
		tnext := tree[d]
		for i := 0; i < nnode; i++ {
			tnext[i] = c.hashNode(nil, tprev[i*2], tprev[i*2+1])
		}
		// If nnode < nhash, just leave the odd one nil.
		nprev = nnext
		tprev = tnext
	}
	if nprev != 1 {
		panic("oops")
	}
	root := tprev[0]

	// Build all the individual proofs from the tree.
	// Some towards the end may end up being shorter than depth.
	proofs := make([]Proof, nleaves)
	for i := 0; i < nleaves; i++ {
		p := make([]HashId, depth)[:0]
		// p = append(p, root)
		for d := depth - 1; d >= 0; d-- {
			h := tree[depth-d][sibling(i>>uint(d))]
			if h != nil {
				p = append(p, h)
			}
		}
		proofs[i] = Proof(p)
	}
	return root, proofs[:nleavesArg]
}

// MerklePath represents a downward path from a (root) node in a Merkle tree
// to a given (interior or leaf) descendant node,
// including all the data necessary to validate and extract the descendant.
// It is assumed the caller has a valid hash-pointer to the root/starting node,
// and that all nodes in the path can be retrieved via self-certifying hash-ID.
type MerklePath struct {
	Ptr []int // Offsets of hash-pointers at each intermediate level
	Ofs int   // Offset of relevant object in last-level blob
	Len int   // Length of relevant object in last-level blob
}

// Retrieve an object in a Merkle tree,
// validating the entire path in the process.
// Returns a slice of a buffer obtained from HashGet.Get(),
// which might be shared and should be considered read-only.
func MerkleGet(suite abstract.Suite, root []byte, path MerklePath,
	ctx HashGet) ([]byte, error) {

	// Follow pointers through intermediate levels
	blob := root
	for i := range path.Ptr {
		beg := path.Ptr[i]
		// end := beg + suite.HashLen()
		end := beg + 256 // change me: find hash len
		if end > len(blob) {
			return nil, errors.New("bad Merkle tree pointer offset")
		}
		id := HashId(blob[beg:end])
		b, e := ctx.Get(id) // Lookup the next-level blob
		if e != nil {
			return nil, e
		}
		blob = b
	}

	// Validate and extract the actual object
	beg := path.Ofs
	end := beg + path.Len
	if end > len(blob) {
		return nil, errors.New("bad Merkle tree object offset/length")
	}
	return blob[beg:end], nil
}

//type MerkleLog struct {
//}
