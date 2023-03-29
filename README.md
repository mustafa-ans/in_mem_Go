# README

This is a key-value datastore API written in Go that can be interacted with using Postman. The API supports the following endpoints:

    /set: Sets a key-value pair in the datastore with optional parameters for expiration and conditional operations (NX, XX).
    /get: Retrieves the value associated with a given key.
    /qpush: Pushes one or more values onto the end of a queue associated with a given key.
    /getall: Retrieves all non-expired key-value pairs in the datastore.

## Usage

    Clone the repository and navigate to the project directory.
    Run the API using the go run command: go run main.go
    Open Postman and import the datastore.postman_collection.json file.
    Send requests to the desired endpoint by selecting it from the Postman collection.

## Endpoint Details
/set

    Method: POST
    Request body format: {"command": "<key> <value> [EX <expiry>] [NX|XX]"} where key is the name of the key, value is the value to be associated with the key, expiry is an optional parameter to specify the time-to-live (TTL) for the key-value pair, and NX or XX are optional parameters to perform a conditional set operation (NX = only set the key if it does not already exist, XX = only set the key if it already exists).
    Response body format (successful operation): {"message": "OK"}
    Response status codes: 200 (successful operation), 400 (invalid command), 409 (key already exists), 500 (internal server error)

## /get

    Method: GET
    Request URL format: /get?key=<key>
    Response body format (successful operation): {"value": "<value>"} where value is the value associated with the key.
    Response status codes: 200 (successful operation), 400 (invalid key), 404 (key not found), 500 (internal server error)

## /qpush

    Method: POST
    Request body format: {"command": "<key> <value1> [<value2> ...]"} where key is the name of the key, and value1, value2, etc. are the values to be pushed onto the queue associated with the key.
    Response body format (successful operation): {"message": "OK"}
    Response status codes: 200 (successful operation), 400 (invalid command), 500 (internal server error)



## /getall

    Method: GET
    Request URL format: /getall
    Response body format (successful operation): {"<key>": "<value>", "<key>": "<value>", ...} where <key> is the name of the key and <value> is the value associated with the key.
    Response status codes: 200 (successful operation), 500 (internal server error)

### To use the code in Go with Postman, you can use the following request bodies for each endpoint:

    /set - POST

    {
    "command": "key value"
    }

    You can also include optional parameters to set the expiration time and check if the key already exists:

    {
    "command": "key value EX 10M NX"
    }
        EX: Sets the expiration time. In this example, the key will expire after 10 minutes.
        NX: Only sets the value if the key does not exist.

    /get - GET

    To get the value of a specific key:

    http://localhost:8080/get?key=key

    /qpush - POST

    {
    "key": "key",
    "values": ["value1", "value2", "value3"]
    }

    /getall - GET

    To get all key-value pairs:

    http://localhost:8080/getall

    To get all key-value pairs that have not expired:

    http://localhost:8080/getall?exp=true
