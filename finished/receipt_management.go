/*
Copyright DASE@ECNU. 2016 All Rights Reserved.
*/

package main

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	//"github.com/op/go-logging"
)

//var myLogger = logging.MustGetLogger("asset_mgm")

type AssetManagementChaincode struct {
}

// The deploy transaction metadata is supposed to contain the warehouse cert
func (t *AssetManagementChaincode) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	fmt.Printf("Init Chaincode...")
	if len(args) != 0 {
		return nil, errors.New("Incorrect number of arguments. Expecting 0")
	}

	// Create ownership table
	err := stub.CreateTable("AssetsOwnership", []*shim.ColumnDefinition{
		&shim.ColumnDefinition{Name: "Id", Type: shim.ColumnDefinition_STRING, Key: true},
		&shim.ColumnDefinition{Name: "Receipt", Type: shim.ColumnDefinition_STRING, Key: false},
		&shim.ColumnDefinition{Name: "Owner", Type: shim.ColumnDefinition_BYTES, Key: false},
	})
	if err != nil {
		return nil, errors.New("Failed creating AssetsOnwership table.")
	}

	return nil, nil
}

func (t *AssetManagementChaincode) setCert(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	fmt.Printf("Set Cert...")
	if len(args) != 2 {
		return nil, errors.New("Incorrect number of arguments. Expecting 2")
	}

	// Set the warehouse
	warehouseCert, err := base64.StdEncoding.DecodeString(args[0])
	if err != nil {
		return nil, errors.New("Failed decoding warehouseCert")
	}

	fmt.Printf("The warehouse is [% x]", warehouseCert)

	stub.PutState("warehouse", warehouseCert)
	
	// Set the warehouse2
    warehouseCert2, err := base64.StdEncoding.DecodeString(args[1])
	if err != nil {
		return nil, errors.New("Failed decoding warehouseCert2")
	}

	fmt.Printf("The warehouse2 is [% x]", warehouseCert2)

	stub.PutState("warehouse2", warehouseCert2)

	fmt.Printf("Set Cert...done")

	return nil, nil
}

func (t *AssetManagementChaincode) assign(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	fmt.Printf("Assign...")

	if len(args) != 4 {
		return nil, errors.New("Incorrect number of arguments. Expecting 4")
	}

	receiptId := args[0]
	receiptJson := args[1]
	invokerStr := args[2]
	owner, err := base64.StdEncoding.DecodeString(args[3])
	if err != nil {
		return nil, errors.New("Failed decoding owner")
	}

	// Verify the identity of the caller
	// Only an warehouse can invoker assign
	warehouseCertificate, err := stub.GetState(invokerStr)
	if err != nil {
		return nil, errors.New("Failed fetching warehouse identity")
	}

	ok, err := t.isCaller(stub, warehouseCertificate)
	if err != nil {
		return nil, errors.New("Failed checking warehouse identity")
	}
	if !ok {
		return nil, errors.New("The caller is not a warehouse")
	}

	// Register assignment
	fmt.Printf("New owner of [%s] is [% x]", receiptId, owner)

	ok, err = stub.InsertRow("AssetsOwnership", shim.Row{
		Columns: []*shim.Column{
			&shim.Column{Value: &shim.Column_String_{String_: receiptId}},
			&shim.Column{Value: &shim.Column_String_{String_: receiptJson}},
			&shim.Column{Value: &shim.Column_Bytes{Bytes: owner}}},
	})

	if !ok && err == nil {
		return nil, errors.New("Receipt has already existed.")
	}

	fmt.Printf("Assign...done!")

	return nil, err
}

func (t *AssetManagementChaincode) transfer(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	fmt.Printf("Transfer...")

	if len(args) != 3 {
		return nil, errors.New("Incorrect number of arguments. Expecting 3")
	}

	asset := args[0]
	receiptJson := args[1]
	newOwner, err := base64.StdEncoding.DecodeString(args[2])
	if err != nil {
		return nil, fmt.Errorf("Failed decoding owner")
	}

	// Verify the identity of the caller
	// Only the owner can transfer one of his assets
	var columns []shim.Column
	col1 := shim.Column{Value: &shim.Column_String_{String_: asset}}
	columns = append(columns, col1)

	row, err := stub.GetRow("AssetsOwnership", columns)
	if err != nil {
		return nil, fmt.Errorf("Failed retrieving asset [%s]: [%s]", asset, err)
	}

	prvOwner := row.Columns[2].GetBytes()
	fmt.Printf("Previous owener of [%s] is [% x]", asset, prvOwner)
	if len(prvOwner) == 0 {
		return nil, fmt.Errorf("Invalid previous owner. Nil")
	}

	// Verify ownership
	ok, err := t.isCaller(stub, prvOwner)
	if err != nil {
		return nil, errors.New("Failed checking asset owner identity")
	}
	if !ok {
		return nil, errors.New("The caller is not the owner of the asset")
	}

	// At this point, the proof of ownership is valid, then register transfer
	err = stub.DeleteRow(
		"AssetsOwnership",
		[]shim.Column{shim.Column{Value: &shim.Column_String_{String_: asset}}},
	)
	if err != nil {
		return nil, errors.New("Failed deliting row.")
	}

	_, err = stub.InsertRow(
		"AssetsOwnership",
		shim.Row{
			Columns: []*shim.Column{
				&shim.Column{Value: &shim.Column_String_{String_: asset}},
				&shim.Column{Value: &shim.Column_String_{String_: receiptJson}},
				&shim.Column{Value: &shim.Column_Bytes{Bytes: newOwner}},
			},
		})
	if err != nil {
		return nil, errors.New("Failed inserting row.")
	}

	err = stub.SetEvent(args[0], []byte(args[1]))
    if err != nil {
        return nil, errors.New("Failed setting event.")
    }

	fmt.Printf("New owner of [%s] is [% x]", asset, newOwner)

	fmt.Printf("Transfer...done")

	return nil, nil
}

func (t *AssetManagementChaincode) isCaller(stub shim.ChaincodeStubInterface, certificate []byte) (bool, error) {
	fmt.Printf("Check caller...")

	// In order to enforce access control, we require that the
	// metadata contains the signature under the signing key corresponding
	// to the verification key inside certificate of
	// the payload of the transaction (namely, function name and args) and
	// the transaction binding (to avoid copying attacks)

	// Verify \sigma=Sign(certificate.sk, tx.Payload||tx.Binding) against certificate.vk
	// \sigma is in the metadata

	sigma, err := stub.GetCallerMetadata()
	if err != nil {
		return false, errors.New("Failed getting metadata")
	}
	payload, err := stub.GetPayload()
	if err != nil {
		return false, errors.New("Failed getting payload")
	}
	binding, err := stub.GetBinding()
	if err != nil {
		return false, errors.New("Failed getting binding")
	}

	fmt.Printf("passed certificate [% x]", certificate)
	fmt.Printf("passed sigma [% x]", sigma)
	fmt.Printf("passed payload [% x]", payload)
	fmt.Printf("passed binding [% x]", binding)

	ok, err := stub.VerifySignature(
		certificate,
		sigma,
		append(payload, binding...),
	)
	if err != nil {
		fmt.Printf("Failed checking signature [%s]", err)
		return ok, err
	}
	if !ok {
		fmt.Printf("Invalid signature!!!")
		return ok, err
	}

	fmt.Printf("Check caller...Verified!")

	return ok, err
}

// Invoke will be called for every transaction.
// Supported functions are the following:
// "assign(asset, owner)": to assign ownership of assets. An asset can be owned by a single entity.
// Only an administrator can call this function.
// "transfer(asset, newOwner)": to transfer the ownership of an asset. Only the owner of the specific
// asset can call this function.
// An asset is any string to identify it. An owner is representated by one of his ECert/TCert.
func (t *AssetManagementChaincode) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {

	// Handle different functions
	if function == "assign" {
		// Assign ownership
		return t.assign(stub, args)
	} else if function == "transfer" {
		// Transfer ownership
		return t.transfer(stub, args)
	} else if function == "setCert" {
		// Transfer ownership
		return t.setCert(stub, args)
	}

	return nil, errors.New("Received unknown function invocation")
}

// Query callback representing the query of a chaincode
// Supported functions are the following:
// "query(asset)": returns the owner of the asset.
// Anyone can invoke this function.
func (t *AssetManagementChaincode) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	fmt.Printf("Query [%s]", function)

	if function != "query" {
		return nil, errors.New("Invalid query function name. Expecting 'query' but found '" + function + "'")
	}

	var err error

	if len(args) != 2 {
		fmt.Printf("Incorrect number of arguments. Expecting 2")
		return nil, errors.New("Incorrect number of arguments. Expecting 2")
	}

	// Who is the owner of the asset?
	asset := args[1]

	fmt.Printf("Arg [%s]", string(asset))

		var columns []shim.Column
		col1 := shim.Column{Value: &shim.Column_String_{String_: asset}}
		columns = append(columns, col1)

		row, err := stub.GetRow("AssetsOwnership", columns)
		if err != nil {
			fmt.Printf("Failed retriving receiptId [%s]: [%s]", string(asset), err)
			return nil, fmt.Errorf("Failed retriving receiptId [%s]: [%s]", string(asset), err)
		}

		if len(row.Columns)==0 {
		    fmt.Printf("No row in result set for receiptId=%s", string(asset))
			return nil, fmt.Errorf("No row in result set for receiptId=%s", string(asset))
		}

		fmt.Printf("row=", row)

	    if args[0] == "getOwner" {
		fmt.Printf("Query done. row.Columns[2]=[% x]", row.Columns[2].GetBytes())
	    return row.Columns[2].GetBytes(), nil
    }else{
    	fmt.Printf("Query done. row.Columns[1]=[%s]", row.Columns[1])
    	return []byte(row.Columns[1].GetString_()), nil
    }
}

func main() {
	primitives.SetSecurityLevel("SHA3", 256)
	err := shim.Start(new(AssetManagementChaincode))
	if err != nil {
		fmt.Printf("Error starting AssetManagementChaincode: %s", err)
	}
}
