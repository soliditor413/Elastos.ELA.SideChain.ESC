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
const address = "0x47CcCf3216BA074343230984340622204ce83FAE";

module.exports = {
    abi,
    address,
};
