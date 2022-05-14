/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	gwproto "github.com/hyperledger/fabric-protos-go/gateway"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strconv"
	"time"
)

const (
	mspID         = "Org1MSP"
	cryptoPath    = "../../../fabric-samples-mod/test-network/organizations/peerOrganizations/org1.example.com"
	certPath      = cryptoPath + "/users/User1@org1.example.com/msp/signcerts/cert.pem"
	keyPath       = cryptoPath + "/users/User1@org1.example.com/msp/keystore/"
	tlsCertPath   = cryptoPath + "/peers/peer0.org1.example.com/tls/ca.crt"
	peerEndpoint  = "localhost:7051"
	gatewayPeer   = "peer0.org1.example.com"
	channelName   = "mychannel"
	chaincodeName = "passport"
)

type Person struct {
	ID      string `json:"id"`
	Serial  string `json:"passport"`
	Name    string `json:"name"`
	Surname string `json:"surname"`
	City    string `json:"city"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Married bool   `json:"married"`
}

type Update struct {
	Tx        string    `json:"tx"`
	Timestamp time.Time `json:"timestamp"`
	Data      *Person   `json:"data"`
}

func main() {
	log.Println("============ application-golang starts ============")

	// The gRPC client connection should be shared by all Gateway connections to this endpoint
	clientConnection := newGrpcConnection()
	defer clientConnection.Close()

	id := newIdentity()
	sign := newSign()

	// Create a Gateway connection for a specific client identity
	gateway, err := client.Connect(
		id,
		client.WithSign(sign),
		client.WithClientConnection(clientConnection),
		// Default timeouts for different gRPC calls
		client.WithEvaluateTimeout(5*time.Second),
		client.WithEndorseTimeout(15*time.Second),
		client.WithSubmitTimeout(5*time.Second),
		client.WithCommitStatusTimeout(1*time.Minute),
	)
	if err != nil {
		panic(err)
	}
	defer gateway.Close()

	network := gateway.GetNetwork(channelName)
	contract := network.GetContract(chaincodeName)

	printHelp()
	for {
		fmt.Print("\ncmd: ")
		var cmd int
		fmt.Scanf("%d", &cmd)
		switch cmd {
		case 9:
			return
		case 1:
			createPerson(contract)
		case 2:
			getAllPersons(contract)
		case 3:
			fmt.Print("Enter id: ")
			var personId string
			fmt.Scanf("%s", &personId)
			personBytes := readPersonByID(contract, personId)
			if len(personBytes) != 0 {
				fmt.Println(formatJSON(personBytes))
			}
		case 4:
			fmt.Print("Enter id: ")
			var personId string
			fmt.Scanf("%s", &personId)
			updatePerson(contract, personId)
		case 5:
			fmt.Print("Enter id: ")
			var personId string
			fmt.Scanf("%s", &personId)
			getPersonHistory(contract, personId)
		default:
			println("Unknown cmd! Try one more time")
			printHelp()
		}
	}

}

func printHelp() {
	fmt.Println("1 - create ")
	fmt.Println("2 - getAll ")
	fmt.Println("3 - getByID ")
	fmt.Println("4 - update ")
	fmt.Println("5 - getHistory ")
	fmt.Println("9 - exit ")
}

// newGrpcConnection creates a gRPC connection to the Gateway server.
func newGrpcConnection() *grpc.ClientConn {
	certificate, err := loadCertificate(tlsCertPath)
	if err != nil {
		panic(err)
	}

	certPool := x509.NewCertPool()
	certPool.AddCert(certificate)
	transportCredentials := credentials.NewClientTLSFromCert(certPool, gatewayPeer)

	connection, err := grpc.Dial(peerEndpoint, grpc.WithTransportCredentials(transportCredentials))
	if err != nil {
		panic(fmt.Errorf("failed to create gRPC connection: %w", err))
	}

	return connection
}

// newIdentity creates a client identity for this Gateway connection using an X.509 certificate.
func newIdentity() *identity.X509Identity {
	certificate, err := loadCertificate(certPath)
	if err != nil {
		panic(err)
	}

	id, err := identity.NewX509Identity(mspID, certificate)
	if err != nil {
		panic(err)
	}

	return id
}

func loadCertificate(filename string) (*x509.Certificate, error) {
	certificatePEM, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file: %w", err)
	}
	return identity.CertificateFromPEM(certificatePEM)
}

// newSign creates a function that generates a digital signature from a message digest using a private key.
func newSign() identity.Sign {
	files, err := ioutil.ReadDir(keyPath)
	if err != nil {
		panic(fmt.Errorf("failed to read private key directory: %w", err))
	}
	privateKeyPEM, err := ioutil.ReadFile(path.Join(keyPath, files[0].Name()))

	if err != nil {
		panic(fmt.Errorf("failed to read private key file: %w", err))
	}

	privateKey, err := identity.PrivateKeyFromPEM(privateKeyPEM)
	if err != nil {
		panic(err)
	}

	sign, err := identity.NewPrivateKeySign(privateKey)
	if err != nil {
		panic(err)
	}

	return sign
}

/*
 This type of transaction would typically only be run once by an application the first time it was started after its
 initial deployment. A new version of the chaincode deployed later would likely not need to run an "init" function.
*/
func initLedger(contract *client.Contract) {
	fmt.Printf("Submit Transaction: InitLedger, function creates the initial set of assets on the ledger \n")

	_, err := contract.SubmitTransaction("InitLedger")
	if err != nil {
		panic(fmt.Errorf("failed to submit transaction: %w", err))
	}

	fmt.Printf("*** Transaction committed successfully\n")
}

func checkPersonExists(contract *client.Contract, personId string) bool {
	evaluateResult, err := contract.EvaluateTransaction("PersonExists", personId)
	if err != nil {
		panic(fmt.Errorf("failed to evaluate transaction: %w", err))
	}
	var res bool
	json.Unmarshal(evaluateResult, &res)
	return res
}

func parsePersonInputCreate(contract *client.Contract) Person {

	fmt.Println("Input Person Data to Create.")

	var p Person
	var input string
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("Id: ")
		p.ID = readLine(scanner)
		if checkPersonExists(contract, p.ID) {
			fmt.Println("Person with this ID already exists! Try another")
		} else {
			break
		}
	}

	for {
		fmt.Print("Serial: ")
		input = readLine(scanner)
		if len(input) == 0 {
			fmt.Printf("required field!\n")
		} else {
			p.Serial = input
			break
		}
	}

	for {
		fmt.Print("Name: ")
		input = readLine(scanner)
		if len(input) == 0 {
			fmt.Printf("required field!\n")
		} else {
			p.Name = input
			break
		}
	}

	for {
		fmt.Print("Surname: ")
		input = readLine(scanner)
		if len(input) == 0 {
			fmt.Printf("required field!\n")
		} else {
			p.Surname = input
			break
		}
	}

	for {
		fmt.Print("City: ")
		input = readLine(scanner)
		if len(input) == 0 {
			fmt.Printf("required field!\n")
		} else {
			p.City = input
			break
		}
	}

	for {
		fmt.Print("Address: ")
		input = readLine(scanner)
		if len(input) == 0 {
			fmt.Printf("required field!\n")
		} else {
			p.Address = input
			break
		}
	}

	for {
		fmt.Print("Phone: ")
		input = readLine(scanner)
		if len(input) == 0 {
			fmt.Printf("required field!\n")
		} else {
			p.Phone = input
			break
		}
	}

	for {
		fmt.Print("Married?: ")
		input = readLine(scanner)
		if len(input) == 0 {
			fmt.Printf("required field!\n")
		} else {
			var err error
			p.Married, err = strconv.ParseBool(input)
			if err != nil {
				fmt.Println("Invalid input! Try ine more time")
			} else {
				break
			}
		}
	}

	return p
}

func parsePersonInputUpdate(p Person) Person {

	fmt.Println("Input Person Data to Update.")
	fmt.Println("To keep current value leave blank input")

	var input string
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("Serial: ", p.Serial, "\nnew value: ")
	input = readLine(scanner)
	if len(input) != 0 {
		p.Serial = input
	}

	fmt.Print("name: ", p.Name, "\nnew value: ")
	input = readLine(scanner)
	if len(input) != 0 {
		p.Name = input
	}

	fmt.Print("surname: ", p.Surname, "\nnew value: ")
	input = readLine(scanner)
	if len(input) != 0 {
		p.Surname = input
	}

	fmt.Print("city:", p.City, "\nnew value: ")
	input = readLine(scanner)
	if len(input) != 0 {
		p.City = input
	}

	fmt.Print("address:", p.Address, "\nnew value: ")
	input = readLine(scanner)
	if len(input) != 0 {
		p.Address = input
	}

	fmt.Print("phone:", p.Phone, "\nnew value: ")
	input = readLine(scanner)
	if len(input) != 0 {
		p.Phone = input
	}
	for {
		fmt.Println("married?:", p.Married, "\nnew value: ")
		input = readLine(scanner)
		if len(input) != 0 {
			var err error
			p.Married, err = strconv.ParseBool(input)
			if err != nil {
				fmt.Println("Invalid input! Try ine more time")
			} else {
				break
			}
		} else {
			break
		}
	}

	return p
}

func createPerson(contract *client.Contract) {
	p := parsePersonInputCreate(contract)

	fmt.Println("Committing to blockchain...")
	_, err := contract.SubmitTransaction("CreatePerson", p.ID, p.Serial, p.Name, p.Surname, p.City, p.Address, p.Phone, strconv.FormatBool(p.Married))
	if err != nil {
		panic(fmt.Errorf("failed to submit transaction: %w", err))
	}

	fmt.Printf("*** Transaction committed successfully\n")
}
func updatePerson(contract *client.Contract, personId string) {

	var person Person
	personBytes := readPersonByID(contract, personId)
	if len(personBytes) == 0 {
		return
	}

	json.Unmarshal(personBytes, &person)
	p := parsePersonInputUpdate(person)

	fmt.Println("Committing to blockchain...")
	_, err := contract.SubmitTransaction("UpdatePerson", p.ID, p.Serial, p.Name, p.Surname, p.City, p.Address, p.Phone, strconv.FormatBool(p.Married))
	if err != nil {
		panic(fmt.Errorf("failed to submit transaction: %w", err))
	}

	fmt.Printf("*** Transaction committed successfully\n")
}

// Evaluate a transaction to query ledger state.
func getAllPersons(contract *client.Contract) {
	fmt.Println("Evaluate Transaction: GetAllPersons, function returns all the current assets on the ledger")

	evaluateResult, err := contract.EvaluateTransaction("GetAllPersons")
	if err != nil {
		panic(fmt.Errorf("failed to evaluate transaction: %w", err))
	}

	if len(evaluateResult) == 0 {
		fmt.Println("database is empty!")
	} else {
		fmt.Printf("*** Result:%s", formatJSON(evaluateResult))
	}
}

// Evaluate a transaction by assetID to query ledger state.
func readPersonByID(contract *client.Contract, personId string) []byte {
	fmt.Printf("Evaluate Transaction: ReadPerson, function returns person attributes\n")

	evaluateResult, err := contract.EvaluateTransaction("ReadPerson", personId)
	if err != nil {
		fmt.Printf("failed to evaluate transaction: %s\n", err)
	}

	return evaluateResult
}

func getPersonHistory(contract *client.Contract, personId string) {
	fmt.Println("Evaluate Transaction: GetPersonHistory, function returns all the current assets on the ledger")

	evaluateResult, err := contract.EvaluateTransaction("GetPersonHistory", personId)
	if err != nil {
		fmt.Printf("failed to evaluate transaction: \n%s\n", err)
		return
	}
	fmt.Println("*** Result:%s", formatJSON(evaluateResult))
}

// Submit transaction, passing in the wrong number of arguments ,expected to throw an error containing details of any error responses from the smart contract.
func exampleErrorHandling(contract *client.Contract) {
	fmt.Println("Submit Transaction: UpdateAsset asset70, asset70 does not exist and should return an error")

	_, err := contract.SubmitTransaction("UpdateAsset")
	if err != nil {
		switch err := err.(type) {
		case *client.EndorseError:
			fmt.Printf("Endorse error with gRPC status %v: %s\n", status.Code(err), err)
		case *client.SubmitError:
			fmt.Printf("Submit error with gRPC status %v: %s\n", status.Code(err), err)
		case *client.CommitStatusError:
			if errors.Is(err, context.DeadlineExceeded) {
				fmt.Printf("Timeout waiting for transaction %s commit status: %s", err.TransactionID, err)
			} else {
				fmt.Printf("Error obtaining commit status with gRPC status %v: %s\n", status.Code(err), err)
			}
		case *client.CommitError:
			fmt.Printf("Transaction %s failed to commit with status %d: %s\n", err.TransactionID, int32(err.Code), err)
		}
		/*
		 Any error that originates from a peer or orderer node external to the gateway will have its details
		 embedded within the gRPC status error. The following code shows how to extract that.
		*/
		statusErr := status.Convert(err)
		for _, detail := range statusErr.Details() {
			errDetail := detail.(*gwproto.ErrorDetail)
			fmt.Printf("Error from endpoint: %s, mspId: %s, message: %s\n", errDetail.Address, errDetail.MspId, errDetail.Message)
		}
	}
}

func readLine(scanner *bufio.Scanner) string {
	scanner.Scan()
	return scanner.Text()
}

//Format JSON data
func formatJSON(data []byte) string {
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, data, " ", ""); err != nil {
		panic(fmt.Errorf("failed to parse JSON: %w", err))
	}
	return prettyJSON.String()
}
