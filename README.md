# nannyapi
This repo is an API endpoint service that receives prompts from nannyagents, do some preprocessing, interact with remote/self-hosted AI APIs to help answering prompts issued by nannyagents.

## Project Structure

```
nannyapi
├── cmd
│   └── main.go        # Entry point of the application
├── internal
│   ├── server
│   │   └── server.go  # Implementation of the server
├── go.mod             # Module definition file
└── README.md          # Documentation for the project
```

## Getting Started

To run the server, navigate to the project directory and execute the following command:

```
go run cmd/main.go
```

## Dependencies

This project uses Go modules for dependency management. Ensure you have Go installed and set up properly.