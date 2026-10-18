package sim

// SplitMix64 is a tiny, fully specified PRNG (Steele, Lea & Flood, 2014).
// It is implemented here rather than taken from math/rand so its sequence is
// pinned by this repository, its whole state is one serialisable uint64, and
// it can be folded into the state hash.
//
// No rule up to rule version 3 consumes randomness; the generator exists so the
// determinism contract (seed in, identical hashes out) is in place before
// rules that need it arrive.
type SplitMix64 struct {
	State uint64
}

// NewSplitMix64 returns a generator seeded with seed.
func NewSplitMix64(seed uint64) SplitMix64 { return SplitMix64{State: seed} }

// Next returns the next 64-bit value.
func (r *SplitMix64) Next() uint64 {
	r.State += 0x9e3779b97f4a7c15
	z := r.State
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// Intn returns a value in [0, n). It panics if n <= 0. The tiny modulo bias
// is acceptable for gameplay use and, more importantly, deterministic.
func (r *SplitMix64) Intn(n int) int {
	if n <= 0 {
		panic("sim: Intn called with n <= 0")
	}
	return int(r.Next() % uint64(n))
}
