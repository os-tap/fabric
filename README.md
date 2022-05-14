# fabric

---
### Запуск тестовой сети
  ```
  ./test-network/network.sh up createChannel -ca
  ```
 
### Деплой контракта
  ```
  ./test-network/network.sh deployCC -ccn passport -ccl go -ccp ./passport/chaincode-go -cci InitLedger
  ```
 

### Консольное приложение
  ```
  cd passport/application-gateway-go
  go run .
  ```
