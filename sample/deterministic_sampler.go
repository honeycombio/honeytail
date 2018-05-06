package sample

import (
	"crypto/sha1"
	"errors"
	"math/big"
)

var (
	ErrInvalidSampleRate = errors.New("sample rate must be >= 1")
	maxVal               = new(big.Int)
)

func init() {
	// maximum possible value for the field value is 160 bytes long
	// (sha1.Size is 20)
	maxVal.Exp(
		big.NewInt(2),
		big.NewInt(int64(sha1.Size*8)),
		big.NewInt(0),
	)
	maxVal.Sub(maxVal, big.NewInt(1))
}

// DeterministicSampler allows for distributed sampling based on a common field
// such as a request or trace ID. It accepts a sample rate N and will
// deterministically sample 1/N events based on the target field. Hence, two or
// more programs can decide whether or not to sample related events without
// communication.
type DeterministicSampler struct {
	sampleRate int
	upperBound *big.Int
}

func NewDeterministicSampler(sampleRate uint) (*DeterministicSampler, error) {
	if sampleRate < 1 {
		return nil, ErrInvalidSampleRate
	}
	upperBound := new(big.Int)

	// Get the actual upper bound - the largest possible value divided by
	// the sample rate. In the case where the sample rate is 1, this should
	// sample every value.
	upperBound.Div(maxVal, big.NewInt(int64(sampleRate)))
	return &DeterministicSampler{
		sampleRate: int(sampleRate),
		upperBound: upperBound,
	}, nil
}

func (ds *DeterministicSampler) Sample(determinant string) bool {
	sum := sha1.Sum([]byte(determinant))
	determinantInt := new(big.Int)
	determinantInt.SetBytes(sum[:])
	cmp := determinantInt.Cmp(ds.upperBound)
	if cmp <= 0 {
		return true
	}

	return false
}
