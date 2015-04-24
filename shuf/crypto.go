package shuf

import (
	"github.com/dedis/crypto/abstract"
	"github.com/dedis/crypto/proof"
	"github.com/dedis/crypto/shuffle"
	"strconv"
)

func createPred(x, y, newY []abstract.Point, b abstract.Point) (
	proof.Predicate, map[string]abstract.Point) {

	// Initialization
	var pred proof.Predicate
	pub := map[string]abstract.Point{}

	// Add requirements for each pair
	for i := range y {
		xname := "X" + strconv.Itoa(i)
		mname := "M" + strconv.Itoa(i)
		yname := "Y" + strconv.Itoa(i)
		pub[xname] = x[i]
		pub[yname] = y[i]
		pub[mname] = newY[i]

		// Create the requirement
		var rep proof.Predicate
		if b != nil {
			rep = proof.Rep(mname, "-h", xname, "1", yname, "r", "B")
		} else {
			rep = proof.Rep(mname, "-h", xname, "1", yname)
		}

		// Add the requirement to the big Predicate
		if pred == nil {
			pred = rep
		} else {
			pred = proof.And(pred, rep)
		}
	}
	pub["B"] = b
	return pred, pub
}

// Encrypt a message with the given public key
func (inf *Info) Encrypt(msgs []abstract.Point, h abstract.Point) (x, y []abstract.Point) {
	rnd := inf.Suite.Cipher(abstract.RandomKey)
	x = make([]abstract.Point, len(msgs))
	y = make([]abstract.Point, len(msgs))
	r := inf.Suite.Secret().Pick(rnd)
	for m := range msgs {
		y[m] = inf.Suite.Point().Mul(h, r)
		y[m] = inf.Suite.Point().Add(y[m], msgs[m])
		x[m] = inf.Suite.Point().Mul(nil, r)
	}
	return x, y
}

// Decrypt a list of pairs with associated proof, potentially adding new encryption
func (inf *Info) Decrypt(x, y, newX []abstract.Point, node int,
	encryptFor abstract.Point) ([]abstract.Point, []abstract.Point, Proof, error) {

	// Initialize
	rnd := inf.Suite.Cipher(abstract.RandomKey)
	newY := make([]abstract.Point, len(y))
	negKey := inf.Suite.Secret().Neg(inf.PrivKey(node))
	r := inf.Suite.Secret().Pick(rnd)

	// Create the Predicate and new pairs
	sec := map[string]abstract.Secret{"-h": negKey, "1": inf.Suite.Secret().One(), "r": r}
	for i := range x {
		newY[i] = inf.Suite.Point().Add(inf.Suite.Point().Mul(x[i], negKey), y[i])
		newY[i] = inf.Suite.Point().Add(newY[i], inf.Suite.Point().Mul(encryptFor, r))
		newX[i] = inf.Suite.Point().Add(newX[i], inf.Suite.Point().Mul(nil, r))
	}
	p, pub := createPred(x, y, newY, encryptFor)

	// Create the proof
	prover := p.Prover(inf.Suite, sec, pub, nil)
	proof, proofErr := proof.HashProve(inf.Suite, "Decrypt", rnd, prover)
	return newX, newY, Proof{x, y, proof}, proofErr
}

// The the combined public key for a bunch of nodes
func (inf *Info) PublicKey(nodes []int) abstract.Point {
	h := inf.Suite.Point().Null()
	for i := len(nodes) - 1; i >= 0; i-- {
		h = inf.Suite.Point().Add(inf.PubKey[nodes[i]], h)
	}
	return h
}

// Verify that the shuffle history from a node is correct
func (inf *Info) VerifyShuffles(history []Proof,
	x, y []abstract.Point, h abstract.Point) error {
	for _, p := range history {
		verifier := shuffle.Verifier(inf.Suite, nil, h, x, y, p.X, p.Y)
		e := proof.HashVerify(inf.Suite, "PairShuffle", verifier, p.Proof)
		if e != nil {
			return e
		}
		y = p.Y
		x = p.X
	}
	return nil
}

// Verify that the decrypt history from a node is correct
func (inf *Info) VerifyDecrypts(history []Proof,
	newY []abstract.Point, encryptFor abstract.Point) error {

	// Check everything but the last proof
	for p := 0; p < len(history)-1; p++ {
		pred, pub := createPred(history[p].X, history[p].Y, history[p+1].Y, encryptFor)
		verifier := pred.Verifier(inf.Suite, pub)
		e := proof.HashVerify(inf.Suite, "Decrypt", verifier, history[p].Proof)
		if e != nil {
			return e
		}
	}

	// Check the last proof
	p := history[len(history)-1]
	pred, pub := createPred(p.X, p.Y, newY, encryptFor)
	verifier := pred.Verifier(inf.Suite, pub)
	e := proof.HashVerify(inf.Suite, "Decrypt", verifier, p.Proof)
	if e != nil {
		return e
	}
	return nil
}