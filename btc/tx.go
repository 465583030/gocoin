package btc

import (
	"fmt"
	"bytes"
	"errors"
	"encoding/hex"
	"encoding/binary"
	"crypto/sha256"
)

const (
	SIGHASH_ALL = 1
	SIGHASH_NONE = 2
	SIGHASH_SINGLE = 3
	SIGHASH_ANYONECANPAY = 0x80
)


type TxPrevOut struct {
	Hash [32]byte
	Vout uint32
}

type TxIn struct {
	Input TxPrevOut
	ScriptSig []byte
	Sequence uint32
	//PrvOut *TxOut  // this field is used only during verification
}

type TxOut struct {
	Value uint64
	BlockHeight uint32
	Pk_script []byte
}

type Tx struct {
	Version uint32
	TxIn []*TxIn
	TxOut []*TxOut
	Lock_time uint32

	// These two fields should be set in block.go:
	Size uint32
	Hash *Uint256
}


type AddrValue struct {
	Value uint64
	Addr20 [20]byte
}


func (to *TxOut) String(ver byte) (s string) {
	s = fmt.Sprintf("%.8f BTC", float64(to.Value)/1e8)
	s += fmt.Sprint(" in block ", to.BlockHeight)
	a := NewAddrFromPkScript(to.Pk_script, ver)
	if a != nil {
		s += " to "+a.String()
	} else {
		s += " pk_scr:" + hex.EncodeToString(to.Pk_script)
	}
	return
}


func (t *Tx) Serialize() ([]byte) {
	var buf [9]byte
	wr := new(bytes.Buffer)

	// Version
	binary.Write(wr, binary.LittleEndian, t.Version)

	//TxIns
	wr.Write(buf[:PutVlen(buf[:], len(t.TxIn))])
	for i := range t.TxIn {
		wr.Write(t.TxIn[i].Input.Hash[:])
		binary.Write(wr, binary.LittleEndian, t.TxIn[i].Input.Vout)
		wr.Write(buf[:PutVlen(buf[:], len(t.TxIn[i].ScriptSig))])
		wr.Write(t.TxIn[i].ScriptSig[:])
		binary.Write(wr, binary.LittleEndian, t.TxIn[i].Sequence)
	}

	//TxOuts
	wr.Write(buf[:PutVlen(buf[:], len(t.TxOut))])
	for i := range t.TxOut {
		binary.Write(wr, binary.LittleEndian, t.TxOut[i].Value)
		wr.Write(buf[:PutVlen(buf[:], len(t.TxOut[i].Pk_script))])
		wr.Write(t.TxOut[i].Pk_script[:])
	}

	//Lock_time
	binary.Write(wr, binary.LittleEndian, t.Lock_time)

	return wr.Bytes()
}


// Return the transaction's hash, that is about to get signed/verified
func (t *Tx) SignatureHash(scriptCode []byte, nIn int, hashType byte) ([]byte) {
	var buf [9]byte

	// Remove any OP_CODESEPARATOR
	var idx int
	var nd []byte
	for idx < len(scriptCode) {
		op, _, n, e := getOpcode(scriptCode[idx:])
		if e!=nil {
			break
		}
		if op != 0xab {
			nd = append(nd, scriptCode[idx:idx+n]...)
		}
		idx+= n
	}
	scriptCode = nd

	ht := hashType&0x1f

	sha := sha256.New()

	binary.LittleEndian.PutUint32(buf[:4], t.Version)
	sha.Write(buf[:4])

	if (hashType&SIGHASH_ANYONECANPAY)!=0 {
		sha.Write([]byte{1}) // only 1 input
		// The one input:
		sha.Write(t.TxIn[nIn].Input.Hash[:])
		binary.LittleEndian.PutUint32(buf[:4], t.TxIn[nIn].Input.Vout)
		sha.Write(buf[:4])
		sha.Write(buf[:PutVlen(buf[:], len(scriptCode))])
		sha.Write(scriptCode[:])
		binary.LittleEndian.PutUint32(buf[:4], t.TxIn[nIn].Sequence)
		sha.Write(buf[:4])
	} else {
		sha.Write(buf[:PutVlen(buf[:], len(t.TxIn))])
		for i := range t.TxIn {
			sha.Write(t.TxIn[i].Input.Hash[:])
			binary.LittleEndian.PutUint32(buf[:4], t.TxIn[i].Input.Vout)
			sha.Write(buf[:4])

			if i==nIn {
				sha.Write(buf[:PutVlen(buf[:], len(scriptCode))])
				sha.Write(scriptCode[:])
			} else {
				sha.Write([]byte{0})
			}

			if (ht==SIGHASH_NONE || ht==SIGHASH_SINGLE) && i!=nIn {
				sha.Write([]byte{0,0,0,0})
			} else {
				binary.LittleEndian.PutUint32(buf[:4], t.TxIn[i].Sequence)
				sha.Write(buf[:4])
			}
		}
	}

	if ht==SIGHASH_NONE {
		sha.Write([]byte{0})
	} else if ht==SIGHASH_SINGLE {
		nOut := nIn
		if nOut >= len(t.TxOut) {
			println("ERROR: SignatureHash(SIGHASH_SINGLE) : out of range in tx",
				nOut, len(t.TxOut), t.Hash.String())
			// Return 1 as the satoshi client (don't ask me why 1, and not something else)
			return []byte{1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0}
		}

		sha.Write(buf[:PutVlen(buf[:], nOut+1)])
		for i:=0; i < nOut; i++ {
			sha.Write([]byte{0xff,0xff,0xff,0xff,0xff,0xff,0xff,0xff,0})
		}
		binary.LittleEndian.PutUint64(buf[:8], t.TxOut[nOut].Value)
		sha.Write(buf[:8])
		sha.Write(buf[:PutVlen(buf[:], len(t.TxOut[nOut].Pk_script))])
		sha.Write(t.TxOut[nOut].Pk_script[:])
	} else {
		sha.Write(buf[:PutVlen(buf[:], len(t.TxOut))])
		for i := range t.TxOut {
			binary.LittleEndian.PutUint64(buf[:8], t.TxOut[i].Value)
			sha.Write(buf[:8])

			sha.Write(buf[:PutVlen(buf[:], len(t.TxOut[i].Pk_script))])

			sha.Write(t.TxOut[i].Pk_script[:])
		}
	}

	binary.LittleEndian.PutUint32(buf[:4], t.Lock_time)
	sha.Write(buf[:4])
	sha.Write([]byte{hashType,0,0,0})
	tmp := sha.Sum(nil)
	sha.Reset()
	sha.Write(tmp)
	return sha.Sum(nil)
}


func (t *TxPrevOut)String() (s string) {
	for i := 0; i<32; i++ {
		s+= fmt.Sprintf("%02x", t.Hash[31-i])
	}
	s+= fmt.Sprintf("-%03d", t.Vout)
	return
}

func (in *TxPrevOut)IsNull() bool {
	return allzeros(in.Hash[:]) && in.Vout==0xffffffff
}


func (tx *Tx) IsCoinBase() bool {
	return len(tx.TxIn)==1 && tx.TxIn[0].Input.IsNull()
}


func (tx *Tx) CheckTransaction() error {
	// Basic checks that don't depend on any context
	if len(tx.TxIn)==0 {
		return errors.New("CheckTransaction() : vin empty")
	}
	if len(tx.TxOut)==0 {
		return errors.New("CheckTransaction() : vout empty")
	}

	// Size limits
	if tx.Size > MAX_BLOCK_SIZE {
		return errors.New("CheckTransaction() : size limits failed")
	}

	if tx.IsCoinBase() {
		if len(tx.TxIn[0].ScriptSig) < 2 || len(tx.TxIn[0].ScriptSig) > 100 {
			return errors.New(fmt.Sprintf("CheckTransaction() : coinbase script size %d",
				len(tx.TxIn[0].ScriptSig)))
		}
	} else {
		for i := range tx.TxIn {
			if tx.TxIn[i].Input.IsNull() {
				return errors.New("CheckTransaction() : prevout is null")
			}
		}
	}

	return nil
}


// Decode a raw transaction output from a given bytes slice.
// Returns the output and the size it took in the buffer.
func NewTxOut(b []byte) (txout *TxOut, offs int) {
	var le, n int

	txout = new(TxOut)

	txout.Value = binary.LittleEndian.Uint64(b[0:8])
	offs = 8

	le, n = VLen(b[offs:])
	if n==0 {
		return nil, 0
	}
	offs+= n

	txout.Pk_script = make([]byte, le)
	copy(txout.Pk_script[:], b[offs:offs+le])
	offs += le

	return
}


// Decode a raw transaction input from a given bytes slice.
// Returns the input and the size it took in the buffer.
func NewTxIn(b []byte) (txin *TxIn, offs int) {
	var le, n int

	txin = new(TxIn)

	copy(txin.Input.Hash[:], b[0:32])
	txin.Input.Vout = binary.LittleEndian.Uint32(b[32:36])
	offs = 36

	le, n = VLen(b[offs:])
	if n==0 {
		return nil, 0
	}
	offs+= n

	txin.ScriptSig = make([]byte, le)
	copy(txin.ScriptSig[:], b[offs:offs+le])
	offs+= le

	// Sequence
	txin.Sequence = binary.LittleEndian.Uint32(b[offs:offs+4])
	offs += 4

	return
}


// Decode a raw transaction from a given bytes slice.
// Returns the transaction and the size it took in the buffer.
// WARNING: This function does not set Tx.Hash neither Tx.Size
func NewTx(b []byte) (tx *Tx, offs int) {
	defer func() { // In case if the buffer was too short, to recover from a panic
		if r := recover(); r != nil {
			println("NewTx failed")
			tx = nil
			offs = 0
		}
	}()

	var le, n int

	tx = new(Tx)

	tx.Version = binary.LittleEndian.Uint32(b[0:4])
	offs = 4

	// TxIn
	le, n = VLen(b[offs:])
	if n==0 {
		return nil, 0
	}
	offs += n
	tx.TxIn = make([]*TxIn, le)
	for i := range tx.TxIn {
		tx.TxIn[i], n = NewTxIn(b[offs:])
		offs += n
	}

	// TxOut
	le, n = VLen(b[offs:])
	if n==0 {
		return nil, 0
	}
	offs += n
	tx.TxOut = make([]*TxOut, le)
	for i := range tx.TxOut {
		tx.TxOut[i], n = NewTxOut(b[offs:])
		offs += n
	}

	tx.Lock_time = binary.LittleEndian.Uint32(b[offs:offs+4])
	offs += 4

	return
}


func (txin *TxIn) GetKeyAndSig() (sig *Signature, key *PublicKey, e error) {
	sig, e = NewSignature(txin.ScriptSig[1:1+txin.ScriptSig[0]])
	if e != nil {
		return
	}
	offs := 1+txin.ScriptSig[0]
	key, e = NewPublicKey(txin.ScriptSig[1+offs:1+offs+txin.ScriptSig[offs]])
	return
}
