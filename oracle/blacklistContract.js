"use strict";

const abi = [
{
    "inputs": [
      {
        "internalType": "address",
        "name": "target",
        "type": "address"
      }
    ],
    "name": "isBlacklisted",
    "outputs": [
      {
        "internalType": "bool",
        "name": "",
        "type": "bool"
      }
    ],
    "stateMutability": "view",
    "type": "function"
},
{
    "inputs": [],
    "name": "mainchain_confiscated_addr",
    "outputs": [
      {
        "internalType": "string",
        "name": "",
        "type": "string"
      }
    ],
    "stateMutability": "view",
    "type": "function"
  }
];

// Default address, can be overridden by consumers if needed.
const address = "0x81a83C24c137774d37382237480B52319E5e05fF";

module.exports = {
    abi,
    address,
};