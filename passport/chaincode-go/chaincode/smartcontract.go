package chaincode

import (
	"encoding/json"
	"fmt"
	"github.com/golang/protobuf/ptypes"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
	"time"
)

// SmartContract provides functions for managing an Person
type SmartContract struct {
	contractapi.Contract
}

// Person describes basic details of what makes up a simple person
// Insert struct field in alphabetic order => to achieve determinism across languages
// golang keeps the order when marshal to json but doesn't order automatically
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
	Data      Person    `json:"data"`
}

// InitLedger adds a base set of persons to the ledger
func (s *SmartContract) InitLedger(ctx contractapi.TransactionContextInterface) error {

	persons := []Person{
		{"person0", "0510 228148", "Igor", "Nikolaev", "Moscow", "Likhachevsky proezd 2", "88005553535", true},
		{"person1", "1020 123654", "Matvei", "Stepanov", "Dolgoprudny", "Universitetskaya 11", "88005553535", false},
	}

	for _, person := range persons {
		personJSON, err := json.Marshal(person)
		if err != nil {
			return err
		}

		err = ctx.GetStub().PutState(person.ID, personJSON)
		if err != nil {
			return fmt.Errorf("failed to put to world state. %v", err)
		}
	}

	return nil
}

// CreatePerson issues a new person to the world state with given details.
func (s *SmartContract) CreatePerson(ctx contractapi.TransactionContextInterface,
	id string,
	serial string,
	name string,
	surname string,
	city string,
	address string,
	phone string,
	married bool) error {

	exists, err := s.PersonExists(ctx, id)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("the person %s already exists", id)
	}

	person := Person{
		ID:      id,
		Serial:  serial,
		Name:    name,
		Surname: surname,
		City:    city,
		Address: address,
		Phone:   phone,
		Married: married,
	}
	personJSON, err := json.Marshal(person)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(id, personJSON)
}

// ReadPerson returns the person stored in the world state with given id.
func (s *SmartContract) ReadPerson(ctx contractapi.TransactionContextInterface, id string) (*Person, error) {
	personJSON, err := ctx.GetStub().GetState(id)
	if err != nil {
		return nil, fmt.Errorf("failed to read from world state: %v", err)
	}
	if personJSON == nil {
		return nil, fmt.Errorf("the person %s does not exist", id)
	}

	var person Person
	err = json.Unmarshal(personJSON, &person)
	if err != nil {
		return nil, err
	}

	return &person, nil
}

// UpdatePerson updates an existing person in the world state with provided parameters.
func (s *SmartContract) UpdatePerson(ctx contractapi.TransactionContextInterface,
	id string,
	serial string,
	name string,
	surname string,
	city string,
	address string,
	phone string,
	married bool) error {
	exists, err := s.PersonExists(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("the person %s does not exist", id)
	}

	// overwriting original person with new person
	person := Person{
		ID:      id,
		Serial:  serial,
		Name:    name,
		Surname: surname,
		City:    city,
		Address: address,
		Phone:   phone,
		Married: married,
	}
	personJSON, err := json.Marshal(person)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(id, personJSON)
}

// DeletePerson deletes an given person from the world state.
func (s *SmartContract) DeletePerson(ctx contractapi.TransactionContextInterface, id string) error {
	exists, err := s.PersonExists(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("the person %s does not exist", id)
	}
	return ctx.GetStub().DelState(id)
}

// PersonExists returns true when person with given ID exists in world state
func (s *SmartContract) PersonExists(ctx contractapi.TransactionContextInterface, id string) (bool, error) {
	personJSON, err := ctx.GetStub().GetState(id)
	if err != nil {
		return false, fmt.Errorf("failed to read from world state: %v", err)
	}

	return personJSON != nil, nil
}

// GetAllPersons returns all persons found in world state
func (s *SmartContract) GetAllPersons(ctx contractapi.TransactionContextInterface) ([]*Person, error) {
	// range query with empty string for startKey and endKey does an
	// open-ended query of all persons in the chaincode namespace.
	resultsIterator, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	var persons []*Person
	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var person Person
		err = json.Unmarshal(queryResponse.Value, &person)
		if err != nil {
			return nil, err
		}
		persons = append(persons, &person)
	}

	return persons, nil
}

func (s *SmartContract) GetPersonHistory(ctx contractapi.TransactionContextInterface, id string) ([]Update, error) {
	exists, err := s.PersonExists(ctx, id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("the person %s does not exist", id)
	}

	resultsIterator, err := ctx.GetStub().GetHistoryForKey(id)
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	var updatesHistory []Update

	for resultsIterator.HasNext() {
		response, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var person Person
		err = json.Unmarshal(response.Value, &person)
		if err != nil {
			return nil, err
		}

		timestamp, err := ptypes.Timestamp(response.Timestamp)
		if err != nil {
			return nil, err
		}

		update := Update{
			Tx:        response.TxId,
			Timestamp: timestamp,
			Data:      person,
		}

		updatesHistory = append(updatesHistory, update)
	}

	return updatesHistory, nil
}
