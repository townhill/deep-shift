package sim

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// Hash is a SHA-256 digest of the complete simulation state.
type Hash [32]byte

func (h Hash) String() string { return hex.EncodeToString(h[:]) }

// hashMagic versions the byte layout below. The layout is part of the replay
// contract: changing it changes every golden hash, so it needs the same
// review as a rule change.
const hashMagic = "deep-shift/state-hash/v3\x00"

// StateHash returns a digest of everything Reset restores plus the immutable
// identity of the run (rule version, seed, dimensions, quota, mechanism
// links). The encoding is explicit fixed-width little-endian, so it is
// identical on every platform and Go version.
func (s *Sim) StateHash() Hash {
	st := &s.st
	b := make([]byte, 0, len(hashMagic)+64+4*len(st.cells))
	b = append(b, hashMagic...)
	b = binary.LittleEndian.AppendUint32(b, RuleVersion)
	b = binary.LittleEndian.AppendUint64(b, s.seed)
	b = binary.LittleEndian.AppendUint64(b, st.tick)
	b = binary.LittleEndian.AppendUint32(b, uint32(s.initial.Width))
	b = binary.LittleEndian.AppendUint32(b, uint32(s.initial.Height))
	b = binary.LittleEndian.AppendUint32(b, uint32(s.initial.RequiredCrystals))
	b = binary.LittleEndian.AppendUint32(b, uint32(st.collected))
	b = binary.LittleEndian.AppendUint32(b, uint32(st.player.X))
	b = binary.LittleEndian.AppendUint32(b, uint32(st.player.Y))
	b = append(b, boolByte(st.exitOpen), byte(st.status), st.moveCooldown)
	b = binary.LittleEndian.AppendUint64(b, st.rng.State)
	for _, c := range st.cells {
		b = append(b, byte(c.Terrain), byte(c.Occupant), boolByte(c.Falling), c.Water)
	}
	b = binary.LittleEndian.AppendUint32(b, uint32(len(s.initial.Mechanisms)))
	for _, m := range s.initial.Mechanisms {
		b = binary.LittleEndian.AppendUint32(b, uint32(m.Valve.X))
		b = binary.LittleEndian.AppendUint32(b, uint32(m.Valve.Y))
		b = binary.LittleEndian.AppendUint32(b, uint32(len(m.Gates)))
		for _, g := range m.Gates {
			b = binary.LittleEndian.AppendUint32(b, uint32(g.X))
			b = binary.LittleEndian.AppendUint32(b, uint32(g.Y))
		}
	}
	return sha256.Sum256(b)
}

func boolByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}
