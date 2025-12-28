// Copyright 2019 The Elastos.ELA.SideChain.ESC Authors
// This file is part of the Elastos.ELA.SideChain.ESC library.
//
// The Elastos.ELA.SideChain.ESC library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Elastos.ELA.SideChain.ESC library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Elastos.ELA.SideChain.ESC library. If not, see <http://www.gnu.org/licenses/>.

// Package blacklist provides a lightweight wrapper to interact with the on-chain
// blacklist contract.
package blacklist

import (
	"errors"
	"strings"

	"github.com/elastos/Elastos.ELA.SideChain.ESC/accounts/abi"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/accounts/abi/bind"
	"github.com/elastos/Elastos.ELA.SideChain.ESC/common"
)

// Minimal ABI containing only the isBlacklisted view method.
const blacklistABI = `[{"inputs":[{"internalType":"address","name":"account","type":"address"}],"name":"isBlacklisted","outputs":[{"internalType":"bool","name":"","type":"bool"}],"stateMutability":"view","type":"function"}]`

// Oracle wraps the blacklist contract to expose a typed Go API.
type Oracle struct {
	address  common.Address
	contract *bind.BoundContract
}

// NewOracle binds the blacklist contract at the given address using the provided backend.
// The backend is expected to implement the read-call path; transactor and filterer can be nil.
func NewOracle(contractAddr common.Address, backend bind.ContractBackend) (*Oracle, error) {
	parsed, err := abi.JSON(strings.NewReader(blacklistABI))
	if err != nil {
		return nil, err
	}
	bound := bind.NewBoundContract(contractAddr, parsed, backend, backend, backend)
	return &Oracle{address: contractAddr, contract: bound}, nil
}

// NewOracleCaller binds the blacklist contract using only a read-only caller backend.
func NewOracleCaller(contractAddr common.Address, caller bind.ContractCaller) (*Oracle, error) {
	parsed, err := abi.JSON(strings.NewReader(blacklistABI))
	if err != nil {
		return nil, err
	}
	bound := bind.NewBoundContract(contractAddr, parsed, caller, nil, nil)
	return &Oracle{address: contractAddr, contract: bound}, nil
}

// ContractAddr returns the contract address.
func (o *Oracle) ContractAddr() common.Address {
	return o.address
}

// IsBlacklisted calls the on-chain isBlacklisted(account) view to check status.
func (o *Oracle) IsBlacklisted(opts *bind.CallOpts, account common.Address) (bool, error) {
	var out []interface{}
	if err := o.contract.Call(opts, &out, "isBlacklisted", account); err != nil {
		return false, err
	}
	if len(out) == 0 {
		return false, errors.New("no return value from isBlacklisted")
	}
	return *abi.ConvertType(out[0], new(bool)).(*bool), nil
}
