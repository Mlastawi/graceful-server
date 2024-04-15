# Server with graceful shutdown demo

This is a showcase of a server with a graceful shutdown.

Run the server, it requires the port `8080` to be available. Then run the client.
To gracefully shut down the server press CTRL-C or send the `SIGINT` signal to server's process.
To forcefully shut down the server send the `SIGKILL` signal to server's process.

Demo parameters can be tweaked by changing constants in the `server.go`.