![qlite logo](/images/qlite.svg)
## About

qlite is a light weight in memory message queue that is intended to hold long-running http requests for asynchronous 
processing. Due to its lightweight nature it is intended to be tightly coupled to the ingesting application. 
DO NOT USE qlite for large scale message handling.

## Install

### Docker
```shell
docker pull sgovindan/qlite:latest

docker run -d -p 8080:8080 qlite:latest
```

### Build from source

Requirements:
- GO version >= 1.22.1

```shell
git clone https://github.com/govindansriram/qlite.git
cd qlite
go build .

## start the server

./qlite start path/to/the/config.yaml
```

## Settings
Settings must be specified in a config.yaml file.

### users (list)
credentials & settings needed to establish a connection

1) name (string): The username of the user
2) password (string): The password of the user
3) publisher (bool) default false: If true user can write messages to the queue
4) subscriber (bool) default false: If true user can read messages from the queue

```yaml
users:
 - name: pub
   password: publisher
   publisher: true
   subscriber: false
```

### port (uint16) default 8080
the port the tcp server will be exposed
```yaml
port: 5000
```

### address (string) default localhost
the address that can be used for communication
```yaml
address: 0.0.0.0
```

### maxSubscriberConnections (uint16) default 2
The amount of subscribers that can be connected to the server
```yaml
maxSubscriberConnections: 10
```

### maxPublisherConnections (uint16) default 1
The amount of publishers that can be connected to the server
```yaml
maxPublisherConnections: 10
```

### maxMessageSize (uint32) default 9437184
The max size of a message in the queue
```yaml
maxMessageSize: 1000
```

### maxIoTimeSeconds (uint32) default 3
The max amount of time waiting for a read or write on a connection to succeed
```yaml
maxIoTimeSeconds: 1000
```

### maxPollingTimeSeconds (uint16) default 10
The max amount of time to wait for LPOP to receive a response from the queue
```yaml
maxIoTimeSeconds: 1000
```

### maxHiddenTimeSeconds (uint16) default 10
The max amount of time a message can be hidden from subscribers for
```yaml
maxIoTimeSeconds: 1000
```

### verbose (bool) default false
display server logs
```yaml
verbose: true
```

## docker YAML config
```yaml
users:
   - name: pub
     password: publisher
     publisher: true

   - name: sub
     password: subscriber
     subscriber: true

port: 8080
maxSubscriberConnections: 5
maxPublisherConnections: 10
maxMessageSize: 10000
maxIoTimeSeconds: 5
maxPollingTimeSeconds: 10
address: 0.0.0.0
maxHiddenTimeSeconds: 30
verbose: true
```

## Communication

Communication takes place over a tcp/ip connection, and must begin with authentication

One common rule is the first 4 bytes of any client or server side message will represent the size of the message being
sent. The size should be an uint32 number in little endian format

## Authentication

1) Initiate the tcp/ip handshake
2) The qlite server will send a greeting packet containing a challenge
    - the packet will be structured with the following byte structure
    - the first bytes contain the size of the message proceeding
    - the next 10 bytes will contain the ascii encoded word 'greetings'
    - this will be followed by a semicolon
    - the following 16 bytes will be an uuid4 string which is the challenge
3) The client response will consist of three parts
    - the first 4 bytes contains the length of the message 
    - the next byte should be an uint8 number this signifies the role with 0 as publisher and 1 as subscriber
    - next the username should be encoded as ascii characters only using alphanumerics, this can take a 
    maximum of 12 bytes. To signify the end of the username use a semicolon as a deliminator.
    - finally the password + challenge should be encrypted using the bcrypt algorithm for 10 rounds
4) if we are able to maintain a connection expect to receive message with the following structure 
   - first 4 bytes will hold the content length of the following message
   - following bytes will contain the ascii encoded message PASS followed by semicolon 
5) Otherwise, expect to receive an error message structured as such 
   - first 4 bytes will hold the content length of the following message
   - then ascii encoded FAIL;
   - then the error message
   - The connection will also automatically close

## Protocol Request

The first 4 bytes of every message are an uint32 segment detailing the size of the message

After that the ascii name for one of 4 different functions should be present followed by a semicolon these are:
1) PUSH: push to the queue can only be done by publishers 
2) HIDE: if a message is available it's data is returned, and the message is hidden for a specified time
(up to ***maxHiddenTimeSeconds***) from subscribers
3) POLL: polls the queue for data until it gets it or until the ***maxPollingTimeSeconds*** times out
4) DEL: deletes a hidden message
5) LEN: gets the approximate length of the queue

### PUSH
after the semicolon add the message you wish to store on the queue. No binary format is required, it is on the consumer to
decipher it.

### HIDE
after the semicolon, the hidden time in seconds must be added as an Uint32 in little endian binary format. The hidden
time must be less than the ***maxHiddenTimeSeconds***

### POLL
after the semicolon, the hidden time in seconds must be added as an Uint32 in little endian binary format. The hidden
time must be less than the ***maxHiddenTimeSeconds***

### DEL
after the semicolon, the next 2 bits should contain the uuid of the hidden message

### LEN
No data is required after the semicolon

## Protocol response

The first 4 bytes of every message are an uint32 segment detailing the size of the message
Following the message length, will be the ascii encoded string PASS or FAIL delimited by a semicolon

### PUSH
- PASS: the position in the queue as a little endian uint32 number
- FAIL: an error message

### HIDE
- PASS: the uuid of the hidden message followed by a semicolon followed by the message
- FAIL: an error message

### POLL
- PASS: the uuid of the hidden message followed by a semicolon followed by the message
- FAIL: an error message

### DEL
- PASS: the uuid of the hidden message
- FAIL: an error message

### LEN
- PASS: how many messages are in the queue as a little endian uint32 number
- FAIL: an error message
