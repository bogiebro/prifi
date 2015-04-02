package shuf

import (
	"fmt"
	"github.com/dedis/crypto/abstract"
	"github.com/dedis/crypto/proof"
	"github.com/dedis/crypto/shuffle"
)

type NeffShuffle struct{}

func (n NeffShuffle) ShuffleStep(pairs Elgamal,
	node int, round int, inf *Info, H abstract.Point) RouteInstr {

	// Shuffle it and decrypt it
	rand := inf.Suite.Cipher(nil)
	xx, yy, prover :=
		shuffle.Shuffle(inf.Suite, nil, H, pairs.X, pairs.Y, rand)
	prf, err := proof.HashProve(inf.Suite, "PairShuffle", rand, prover)
	if err != nil {
		fmt.Printf("Error creating proof: %s\n", err.Error())
	}
	pairs.X = xx
	pairs.Y = yy
	pairs, H = decryptPairs(pairs, inf, node, H)

	// Send it on its way
	instr := RouteInstr{Pairs: pairs, H: H, Proof: prf}
	next := node + 1
	if next < inf.NumNodes {
		instr.To = []int{next}
	}
	return instr
}

func (n NeffShuffle) Setup(msg abstract.Point, client int,
	inf *Info) (Elgamal, abstract.Point, int) {
	X, Y, H := onionEncrypt([]abstract.Point{msg}, inf, xrange(inf.NumNodes))
	elg := Elgamal{X, Y}
	return elg, H, 0
}

func (n NeffShuffle) VerifyShuffle(newPairs, oldPairs Elgamal,
	H abstract.Point, inf *Info, prf []byte) error {
	verifier := shuffle.Verifier(inf.Suite, nil, H, oldPairs.X, oldPairs.Y, newPairs.X, newPairs.Y)
	return proof.HashVerify(inf.Suite, "PairShuffle", verifier, prf)
}

func (n NeffShuffle) ActiveRounds(node int, inf *Info) []int {
	return []int{node}
}
