## NOT SUPPORTED YET
To start logging requests.
```
GET http://a.proxi/api/logging/start
```

To stop Logging requests.
```
GET http://a.proxi/api/logging/stop
```

To erase logged requests.
```
GET http://a.proxi/api/logging/clear
```

To get requests.
```
GET http://a.proxi/api/logging/get
```

Result of a get (Right now in the order of response):
```
[
  {
    "request": { // unmodified request
      "url" : "whatever.com", // the URL of the request
      "headers" : "field: value\n field2: value2", // Just the headers dumped, formatted as in the http protocol.
      "body": "text data here", // This is not supported yet, the string is a placeholder.
      "timestamp": 1234 // This is not here yet, it will be added later.
    },
    "response": { // response to the request, modified.
      "status" : "200 OK", // The status code of the request.
      "headers" : "field: value\n field2: value2", // Just the headers dumped, formatted as in the http protocol.
      "body": "text data here", // This is not supported yet, the string is a placeholder.
      "timestamp": 1235 // This is not here yet, it will be added later.
    }
  },
  {
    "request": { // Same as above, but this request came after the last one.
      "url" : "whatever.com",
      "headers" : "field: value\n field2: value2",
      "body": "text data here",
      "timestamp": 2345
    },
    "response": {
      "status" : "200 OK",
      "headers" : "field: value\n field2: value2",
      "body": "text data here",
      "timestamp": 2346
    }
  }
]
```
