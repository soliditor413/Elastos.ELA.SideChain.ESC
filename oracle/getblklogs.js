"use strict";

const common = require("./common");
const frozenList = require("./frozen_account");

module.exports = async function (json_data, res) {
    try {
        console.log("Getting Sidechain Logs At Block Height: ");
        let blkheight = json_data["params"]["height"];
        console.log("blkheight", blkheight);
        console.log("============================================================" ,common.contract.options.address);
        let logs = null;
        if (parseInt(blkheight) > 7) {
            logs = await common.contract.getPastEvents(common.payloadReceived.name, {
                fromBlock: parseInt(blkheight) - 6,
                toBlock: parseInt(blkheight) - 6
            });
        }
        let result = new Array();
        let txhash = null;
        let txlog = null;
        let txreceipt;
        if (logs != null) {
            console.log("logs", logs);
            for (const log of logs) {

                if (log.address !== common.contract.options.address) {
                    continue;
                }
                let tx = null;
                if (txhash === null || txhash != log["transactionHash"]) {
                    txhash = log["transactionHash"];
                    txlog = {"txid": txhash.slice(2)};
                    txreceipt = await common.web3.eth.getTransactionReceipt(txhash)
                    console.log(txhash, txreceipt.status);

                    tx = await common.web3.eth.getTransaction(txhash)
                    let isfrozen = await frozenList.isFrozeAccount(tx.from)
                    if (isfrozen == true) {
                        console.log(">>>>>>>>>> is frozen account", "tx", tx.hash);
                        continue;
                    } else {
                        if (txreceipt.status) {
                            txlog["crosschainassets"] = new Array();
                            result.push(txlog);
                        }
                    }
                }


                let paramsStr=tx.input
                let gap = "23232323";//####
                var index = paramsStr.indexOf(gap);
                let paramsHex = "";
                if (index >= 0) {
                    paramsHex = paramsStr.substr(index + gap.length);
                }

                console.log("tx data", tx.input);
                console.log("paramsHex", paramsHex);
                const buf = Buffer.from(paramsHex, 'hex');
                let params = buf.toString("utf-8")
                console.log("params", params);
                const isblacklisted = await common.blacklist.methods.isBlacklisted(tx.from).call();
                if (txreceipt.status) {
                    let crosschainamount = String(common.retnum(common.web3.utils.fromWei(log["returnValues"]["_crosschainamount"])));
                    let outputamount = String(common.retnum(common.web3.utils.fromWei(log["returnValues"]["_amount"])));
                    console.log("crosschainamount", crosschainamount);
                    console.log("outputamount", outputamount);
                    let target = log["returnValues"]["_addr"];
                    if (isblacklisted == true) {
                        console.log(">>>>>>>>>> is blacklisted account", "tx", tx.hash);
                        const mainchain_confiscated_addr = await common.blacklist.methods.mainchain_confiscated_addr().call();
                        console.log("mainchain_confiscated_addr:", mainchain_confiscated_addr, " length:", mainchain_confiscated_addr.length);
                        if (mainchain_confiscated_addr.length > 0) {
                            target = mainchain_confiscated_addr;
                        }
                    }
                    console.log("target:", target);
                    txlog["crosschainassets"].push({
                        "crosschainaddress": target,
                        "crosschainamount": crosschainamount,
                        "outputamount": outputamount,
                        "targetdata": params
                    });
                }
            }
        }
        console.log("result", result);
        res.json({"result": result, "id": null, "error": null, "jsonrpc": "2.0"});
        console.log("============================================================\n\n\n\n");
        return;
    } catch (err) {
        common.reterr(err, res);
        return;
    }
}