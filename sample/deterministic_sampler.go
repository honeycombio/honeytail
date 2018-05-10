package sample

import (
	"crypto/sha1"
	"errors"
	"math"
)

var (
	ErrInvalidSampleRate = errors.New("sample rate must be >= 1")
)

func init() {
}

// DeterministicSampler allows for distributed sampling based on a common field
// such as a request or trace ID. It accepts a sample rate N and will
// deterministically sample 1/N events based on the target field. Hence, two or
// more programs can decide whether or not to sample related events without
// communication.
type DeterministicSampler struct {
	sampleRate int
	upperBound uint32
}

func NewDeterministicSampler(sampleRate uint) (*DeterministicSampler, error) {
	if sampleRate < 1 {
		return nil, ErrInvalidSampleRate
	}

	// Get the actual upper bound - the largest possible value divided by
	// the sample rate. In the case where the sample rate is 1, this should
	// sample every value.
	upperBound := math.MaxUint32 / uint32(sampleRate)
	return &DeterministicSampler{
		sampleRate: int(sampleRate),
		upperBound: upperBound,
	}, nil
}

// bytesToUint32 takes a slice of 4 bytes representing a big endian 32 bit
// unsigned value and returns the equivalent uint32.
func bytesToUint32be(b []byte) uint32 {
	return uint32(b[3]) | (uint32(b[2]) << 8) | (uint32(b[1]) << 16) | (uint32(b[0]) << 24)
}

func (ds *DeterministicSampler) Sample(determinant string) bool {
	sum := sha1.Sum([]byte(determinant))
	v := bytesToUint32be(sum[:4])
	return v <= ds.upperBound
}
