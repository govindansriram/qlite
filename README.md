# qlite

## About

qlite is a light weight in memory message queue that is intended to hold long-running http requests for asynchronous 
processing. Due to its lightweight nature it is intended to be tightly coupled to the ingesting application. 
DO NOT USE qlite for large scale message handling.

## Settings

Settings must be specified in a config.yaml file.

Settings include:

### users (list)
A list of users that have permissions to access the queue.

#### name (string)
The username of the user

#### password (string)
The password of the user

#### publisher (bool) default false
If true user can write messages to the queue

#### subscriber (bool) default false
If true user can read messages from the queue

### port (uint16) default 8080
The port the server will be run on

### maxSubscriberConnections (uint16) default 1
The amount of subscribers that can be connected to the server

### maxPublisherConnections (uint16) default 1
The amount of publishers that can be connected to the server

### maxMessages (uint32) default 100
The max amount of messages that can be in the queue at once


```yaml
users:
   - name: user1
     password: mypassword
     publisher: true
     subscriber: false
     
   - name: user2
     password: mypassword2
     publisher: false
     subscriber: true
     
port: 8080
maxSubscriberConnections: 2
maxPublisherConnections: 2
maxMessages: 1000
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
   - following bytes will contain the ascii encoded message pass 
5) Otherwise, expect to receive an error message structured as such 
   - first 4 bytes will hold the content length of the following message
   - ascii encoded error string
   - The connection will also automatically close

## Protocol Request

The first 4 bytes of every message are an uint32 segment detailing the size of the message (excluding these byte)

After that the ascii name for one of 4 different functions should be present followed by a semicolon these are:
1) PUSH: push to the queue can only be done by publishers 
2) SPOP: (short pop), pop from the queue if there is data to pop, else returns error
3) LPOP: (long pop), polls the queue for data until it gets it for x amount of time, if request times out returns error
4) LEN: gets the length of the queue

### PUSH
after the semicolon add the message you wish to store on the queue. No binary format is required, it is on the consumer to
decipher it.

### SPOP
No data is needed after the semicolon

### LPOP
No data is needed after the semicolon

### LEN
No data is required after the semicolon

## Protocol response

The first 4 bytes of every message are an uint32 segment detailing the size of the message (excluding these byte)
Following the message length, will be the ascii encoded string PASS or FAIL delimited by a semicolon

### PUSH
PASS: the position in the queue of the message, will come after the pass
FAIL: an error message

### SPOP
PASS: the message on the queue
FAIL: an error message

### LPOP
PASS: the message on the queue
FAIL: an error message

### LEN
PASS: how many messages are left on the queue
FAIL: an error message