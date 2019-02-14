package cxserver

import (
	"fmt"

	"github.com/mit-dci/lit/btcutil/base58"
	"github.com/mit-dci/lit/wallit"
)

// NewAddressLTC returns a new address based on the keygen retrieved from the wallet
func (server *OpencxServer) NewAddressLTC(username string) (string, error) {
	// No really what is this
	return server.getLTCAddrFunc()(username)
}

// NewAddressBTC returns a new address based on the keygen retrieved from the wallet
func (server *OpencxServer) NewAddressBTC(username string) (string, error) {
	// What is this
	return server.getBTCAddrFunc()(username)
}

// NewAddressVTC returns a new address based on the keygen retrieved from the wallet
func (server *OpencxServer) NewAddressVTC(username string) (string, error) {
	// Is this currying
	return server.getVTCAddrFunc()(username)
}

// getVTCAddrFunc is used by NewAddressVTC as well as UpdateAddresses to call the address closure
func (server *OpencxServer) getVTCAddrFunc() func(string) (string, error) {
	return GetAddrFunction(server.OpencxVTCWallet)
}

// getBTCAddrFunc is used by NewAddressBTC as well as UpdateAddresses to call the address closure
func (server *OpencxServer) getBTCAddrFunc() func(string) (string, error) {
	return GetAddrFunction(server.OpencxBTCWallet)
}

// getLTCAddrFunc is used by NewAddressLTC as well as UpdateAddresses to call the address closure
func (server *OpencxServer) getLTCAddrFunc() func(string) (string, error) {
	return GetAddrFunction(server.OpencxLTCWallet)
}

// GetAddrFunction returns a function that can safely be called by the DB
func GetAddrFunction(wallet *wallit.Wallit) func(string) (string, error) {
	pubKeyHashAddrID := wallet.Param.PubKeyHashAddrID
	return func(username string) (addr string, err error) {
		defer func() {
			if err != nil {
				err = fmt.Errorf("Problem with address closure: \n%s", err)
			}
		}()

		// Create a new address
		var addrBytes [20]byte
		if addrBytes, err = wallet.NewAdr160(); err != nil {
			return
		}

		// encode it to store in own db
		addr = base58.CheckEncode(addrBytes[:], pubKeyHashAddrID)

		return
	}
}
