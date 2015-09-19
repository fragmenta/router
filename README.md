# router
A router which allows pattern matching, routes, redirects, filters and provides a handler interface


### Usage 

Setup the router

```Go 
	// Routing
	router, err := router.New(server.Logger, server)
	if err != nil {
		server.Fatalf("Error creating router %s", err)
	}
```

Add a route with named parameters, matching a regexp, and a method if necessary

```Go 
	r.Add("/tags/{id:[0-9]+}/destroy", tagactions.HandleDestroy).Post()
```




### ContextHandler interface

The router handlers accept a context (which wraps Writer and Request, and provides some other functions), and return an error for easier error handling. The error may contain status codes and other details of the error for reporting in development environments. 


```Go 
	// Setup server
	server, err := server.New()
	if err != nil {
		fmt.Printf("Error creating server %s", err)
		return
	}

	// Write to log 
	server.Logf("#info Starting server in %s mode on port %d", server.Mode(), server.Port())

	// Start the server
	err = server.Start()
	if err != nil {
		server.Fatalf("Error starting server %s", err)
	}
```
