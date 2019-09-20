
### To clear rewrite rules (example)
Request Method (Doesn't matter for now)
```
POST
```
Request URL
```
http://a.proxi/api/rules/set
```
Request Body (JSON)
```
{
	"ip": "127.0.0.1"
	"rules":[]
}
```

### To set rewrite rules (example)
Request Method  (Doesn't matter for now)
```
POST
```
Request URL
```
http://a.proxi/api/rules/set
```
Request Body (JSON)
```
{
    "rules": [
        {
            "url": "google\\.com",
            "uploadSpeed": 16000000,
            "downloadSpeed": 8000000,
            "responseDelay": 10000,
            "rewrite": {
                "request": {
                    "url": [
                        {
	                        "find": "(?i)google",
	                        "replace": "bing"
                        }
                    ],
                    "header": [
                        {
	                        "find": "(?i)\\nDate: .*\\n",
	                        "replace": "\nDate: TODAY\n"
                        }
                    ],
                    "body": [
                        {
	                        "find": "(?i)hello world",
	                        "replace": "restfulHttpsProxy"
                        }
                    ]
                },
                "response": {
	                "status": [
		                {
			                "replace": "418 I'm a teapot"
			            }
					],
                    "header": [
                        {
	                        "find": "(?i)\\nDate: .*\\n",
	                        "replace": "\nDate: TODAY\n"
                        }
                    ],
                    "body": [
                    	{
	                        "find": "bing",
	                        "replace": "google"
                        },
                        {
	                        "find": "(?i)hello world",
	                        "replace": "(restfulHttpsProxy)"
                        }
                    ]
                }
            }
        }
    ]
}
```
- *This command will delete existing rules and use the new ones*
- *The Regular expressions must be double escaped. so the regex `\.` will be `\\.` to look for a dot.*

### Supported Keys
- root object without key
   - **ip** Optional field, specifies the ip that the rules apply to.
   - **rules** Array of proxy rules, can be empty to clear rules
      - **url** Regex that will trigger the application of this rule if it is satisfied when compared to the url
	 - **uploadSpeed** Throttles the upload speed to this value if url pattern is satisfied (Rate is in bits/second)
	 - **downloadSpeed** Throttles the download speed to this value if url pattern is satisfied (Rate is in bits/second)
	 - **responseDelay** Kind of like ping, but what it actually does is it simulates a slow server that thinks for this amount of time before responding.
	 - **rewrite**  All of the rewrite rules that modify traffic go here.
		 - **request**
			 - **url** Array of url rule objects
				 - see rule objects below
			 - **headers** Array of header rule objects
				 - see rule objects below
			 - **body** Array of body rule objects
				 - see rule objects below
		 - **response**
			 - **status** Array of status rule objects
				 - see rule objects below
			 - **headers** Array of header rule objects
				 - see rule objects below
			 - **body** Array of body rule objects
				 - **find** Can only be used with replace (Regex pattern to find)
				 - **replace**  Replaces what is found by find, otherwise will just replace the whole thing. Cannot be used with anything except for **find**
				 - **delete** Deletes every instance of the matched regex pattern, cannot be used with any other key.
				 - **append** Adds this to the end of the data. Cannot be used together with any other key.
				 - **prepend** Adds this to the beginning of the data. Cannot be used together with any other key.

- *The regular expressions must be double escaped. so the regex `\.` will be `\\.` to look for a dot.*
- *The regular expressions are in golang regex format.*
- *if you want to use (**find**  + **replace**)  (**delete**)  (**append**)  (**prepend**) together, then you must separate them into separate rules*
