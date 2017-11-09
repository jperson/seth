package seth

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// EtherType represents a type in the
// Ethereum Contract ABI type system
// https://github.com/ethereum/wiki/wiki/Ethereum-Contract-ABI
type EtherType interface {
	// EncodeABI encodes this type using the
	// canonical contract ABI encoding
	EncodeABI(v []byte) []byte

	internal() // cannot be implemented outside this library
}

// EtherSlice is an EtherType that represents
// a dynamically-sized list (e.g. []address or []uint in solidity)
type EtherSlice interface {
	// The implementation of EtherSlice.EncodeABI should
	// only concatenate the raw slice values to the input,
	// and not the slice length prefix.
	EtherType

	// Len should return the number of elements in the slice
	Len() int
}

// EncodeABI implements EtherType
func (a *Address) EncodeABI(v []byte) []byte {
	var word [32]byte
	copy(word[12:], a[:])
	return append(v, word[:]...)
}

func (a *Address) internal() {}

// EncodeABI implements EtherType
func (i *Int) EncodeABI(v []byte) []byte {
	var w [32]byte
	bits := (*big.Int)(i).Bytes()
	if len(bits) > 32 {
		panic("ABI encoding: integer overflow")
	}
	copy(w[32-len(bits):], bits)
	return append(v, w[:]...)
}

func (i *Int) internal() {}

func (d *Data) EncodeABI(v []byte) []byte {
	if len(*d) > 32 {
		panic("can't encode data with len greater than 32")
	}
	var w [32]byte
	copy(w[:], *d)
	return append(v, w[:]...)
}

func (d *Data) internal() {}

func padint(i uint, v []byte) []byte {
	var w [32]byte
	binary.BigEndian.PutUint64(w[32-8:], uint64(i))
	return append(v, w[:]...)
}

// IntSlice is an implementation of EtherSlice
// for a list of integers
type IntSlice []Int

// EncodeABI implements EtherType.EncodeABI
func (i *IntSlice) EncodeABI(v []byte) []byte {
	for j := range *i {
		v = (*i)[j].EncodeABI(v)
	}
	return v
}

// Len implements EtherSlice.Len
func (i *IntSlice) Len() int  { return len(*i) }
func (i *IntSlice) internal() {}

// AddrSlice is an implementation of EtherSlice
// for a list of addresses
type AddrSlice []Address

func (a *AddrSlice) EncodeABI(v []byte) []byte {
	for j := range *a {
		v = (*a)[j].EncodeABI(v)
	}
	return v
}

// Len implements EtherSlice.Len
func (a *AddrSlice) Len() int  { return len(*a) }
func (a *AddrSlice) internal() {}

// CallOpts describes a transaction (contract call).
type CallOpts struct {
	From     *Address `json:"from,omitempty"`     // Sender address
	To       *Address `json:"to,omitempty"`       // Contract address
	Gas      *Int     `json:"gas,omitempty"`      // Gas offered for call
	GasPrice *Int     `json:"gasPrice,omitempty"` // GasPrice offered for gas
	Value    *Int     `json:"value,omitempty"`    // Value to send
	Data     Data     `json:"data"`               // Input to the call
}

const illegal = " \t\n\b-+/~!@#$%^&*=|;:\"<>\\?"

// check that the given arguments correspond
// to the arguments given in the function signature 'f'
// where 'f' is of the form
//  name(type0,type1,type2)
func typecheck(f string, args []EtherType) {
	if strings.ContainsAny(f, illegal) {
		panic("illegal characters in function signature string")
	}
	lparen := strings.IndexByte(f, '(')
	if lparen == -1 {
		panic(f + " has no left paren")
	}
	rparen := strings.IndexByte(f, ')')
	if rparen != len(f)-1 {
		panic(f + " has a bad right paren")
	}
	var argstrings []string
	if strings.Contains(f[lparen+1:rparen], ",") {
		argstrings = strings.Split(f[lparen+1:rparen], ",")
		if len(argstrings) != len(args) {
			panic(fmt.Sprintf("mismatched argument lists: %d args vs %d given", len(argstrings), len(args)))
		}
	}
	for i := range argstrings {
		switch argstrings[i] {
		case "address":
			if _, ok := args[i].(*Address); !ok {
				panic("address argument not an address")
			}
		case "uint", "uint256", "int", "int256":
			if _, ok := args[i].(*Int); !ok {
				panic(argstrings[i] + " argument not an Int")
			}
		case "bytes32":
			if _, ok := args[i].(*Data); !ok {
				if _, ok = args[i].(*Int); !ok {
					panic(argstrings[i] + "argument not an Int or Data")
				}
			}
		default:
			if strings.HasSuffix(argstrings[i], "[]") {
				if _, ok := args[i].(EtherSlice); !ok {
					panic("argument not a slice")
				}
			}
			// TODO: more typechecking
		}
	}
}

// ABIEncode encodes a function and its arguments
func ABIEncode(fn string, args ...EtherType) []byte {
	typecheck(fn, args)

	buf := make([]byte, 4, 4+len(args)*32)
	fhash := HashString(fn)
	copy(buf[:4], fhash[:4])

	var dyn []EtherSlice
	dynoff := uint(len(args)) * 32
	for _, a := range args {
		if es, ok := a.(EtherSlice); ok {
			// just encode the dynamic argument offset
			buf = padint(dynoff, buf)
			dyn = append(dyn, es)
			dynoff += 32
			continue
		}
		buf = a.EncodeABI(buf)
	}
	for i := range dyn {
		buf = padint(uint(dyn[i].Len()), buf)
		buf = dyn[i].EncodeABI(buf)
	}
	return buf
}

// EncodeCall sets up c.Data so that it reflects
// the given function signature and argument list.
//
// EncodeCall does some rudimentary sanity checking on
// the provided arguments and panics if the function signature
// string or argument list is malformed. For instance,
// for a function signature of "transfer(address,uint256)",
// EncodeCall would panic if two arguments weren't provided,
// or if they weren't an *Address and *Int, respectively.
func (c *CallOpts) EncodeCall(fn string, args ...EtherType) {
	c.Data = Data(ABIEncode(fn, args...))
}

// Call makes a transaction call using the given CallOpts.
func (c *Client) Call(opts *CallOpts) (tx Hash, err error) {
	buf, _ := json.Marshal(opts)
	err = c.do("eth_sendTransaction", []json.RawMessage{buf}, &tx)
	return
}

// EstimateGas estimates the gas cost of mining this call into the blockchain.
func (c *Client) EstimateGas(opts *CallOpts) (gas Int, err error) {
	buf, _ := json.Marshal(opts)
	err = c.do("eth_estimateGas", []json.RawMessage{buf, rawpending}, &gas)
	return
}

// ConstCall executes an EVM call without mining a transaction into the blockchain.
// If 'pending' is true, the transaction is executed in the pending block; otherwise
// the call is executed in the latest block. 'out' should be a type that can be
// unmarshaled from the JSON representation of the return value of the function.
func (c *Client) ConstCall(opts *CallOpts, out interface{}, pending bool) error {
	buf, _ := json.Marshal(opts)
	args := []json.RawMessage{buf, rawlatest}
	if pending {
		args[1] = rawpending
	}
	return c.do("eth_call", args, out)
}

// StorageAt reads contract storage from a contract at a particular 256-bit address.
func (c *Client) StorageAt(addr *Address, offset *Hash, block int64) (Hash, error) {
	buf, _ := json.Marshal(addr)
	buf2, _ := json.Marshal(offset)
	var buf3 []byte
	switch block {
	case -2:
		buf3 = rawpending
	case -1:
		buf3 = rawlatest
	default:
		buf3, _ = json.Marshal(block)
	}
	var out Hash
	err := c.do("eth_getStorageAt", []json.RawMessage{buf, buf2, buf3}, &out)
	return out, err
}