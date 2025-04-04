// Package docs Code generated by swaggo/swag. DO NOT EDIT
package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {
            "name": "API Support",
            "url": "https://nannyai.dev/support",
            "email": "harsha@harshanu.space"
        },
        "license": {
            "name": "GNU General Public License v3.0",
            "url": "https://www.gnu.org/licenses/gpl-3.0.html"
        },
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/api/agent-info/{id}": {
            "get": {
                "description": "Retrieves agent information by ID",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "agent-info"
                ],
                "summary": "Get agent info by ID",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Agent ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Successfully retrieved agent info",
                        "schema": {
                            "$ref": "#/definitions/agent.AgentInfo"
                        }
                    },
                    "400": {
                        "description": "Invalid ID format",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "401": {
                        "description": "User not authenticated",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "404": {
                        "description": "Agent info not found",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Failed to retrieve agent info",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/api/agent-infos": {
            "post": {
                "description": "Ingest agent information",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "agent-info"
                ],
                "summary": "Ingest agent information",
                "parameters": [
                    {
                        "description": "Agent Information",
                        "name": "agentInfo",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/agent.AgentInfo"
                        }
                    }
                ],
                "responses": {
                    "201": {
                        "description": "id of the inserted agent info",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "400": {
                        "description": "Invalid request payload",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "401": {
                        "description": "User not authenticated",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Failed to create agent",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/api/agents": {
            "get": {
                "description": "Ingest agent information",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "agent-info"
                ],
                "summary": "Ingest agent information",
                "responses": {
                    "200": {
                        "description": "Successfully retrieved agent info",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/agent.AgentInfo"
                            }
                        }
                    },
                    "400": {
                        "description": "Invalid request payload",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "401": {
                        "description": "User not authenticated",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Failed to retrieve agents info",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/api/auth-token": {
            "post": {
                "description": "Creates auth token (aka API key) for the authenticated user.",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "auth-token"
                ],
                "summary": "Creates auth token (aka API key) for the authenticated user",
                "responses": {
                    "201": {
                        "description": "id of the inserted token",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "400": {
                        "description": "Invalid request payload",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "401": {
                        "description": "User not authenticated",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Failed to create API key",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/api/auth-token/{id}": {
            "delete": {
                "description": "Deletes a specific auth token by ID.",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "auth-tokens"
                ],
                "summary": "Delete an auth token",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Token ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Auth token deleted successfully",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "400": {
                        "description": "Invalid token ID format or Token ID is required",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Failed to delete auth token",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/api/auth-tokens": {
            "get": {
                "description": "Retrieves all auth tokens for the authenticated user.",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "auth-tokens"
                ],
                "summary": "Get all auth tokens",
                "responses": {
                    "200": {
                        "description": "Successfully retrieved auth tokens",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/token.Token"
                            }
                        }
                    },
                    "401": {
                        "description": "User not authenticated",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Failed to retrieve auth tokens",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/api/chat": {
            "post": {
                "description": "Starts a new chat session",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "chat"
                ],
                "summary": "Start a new chat session",
                "parameters": [
                    {
                        "description": "Agent ID",
                        "name": "agentID",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "type": "string"
                        }
                    }
                ],
                "responses": {
                    "201": {
                        "description": "Chat session started successfully",
                        "schema": {
                            "$ref": "#/definitions/chat.Chat"
                        }
                    },
                    "500": {
                        "description": "Failed to start chat session",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/api/chat/{id}": {
            "get": {
                "description": "Retrieves a chat session by ID",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "chat"
                ],
                "summary": "Get a chat session by ID",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Chat ID",
                        "name": "chatID",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Successfully retrieved chat session",
                        "schema": {
                            "$ref": "#/definitions/chat.Chat"
                        }
                    },
                    "400": {
                        "description": "Invalid chat ID format",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "404": {
                        "description": "Chat session not found",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Failed to retrieve chat session",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            },
            "put": {
                "description": "Adds a prompt-response pair to an existing chat session",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "chat"
                ],
                "summary": "Add a prompt-response pair to a chat session",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Chat ID",
                        "name": "chatID",
                        "in": "path",
                        "required": true
                    },
                    {
                        "description": "Prompt and Response",
                        "name": "promptResponse",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/chat.PromptResponse"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Prompt-response pair added successfully",
                        "schema": {
                            "$ref": "#/definitions/chat.Chat"
                        }
                    },
                    "400": {
                        "description": "Invalid request payload",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "404": {
                        "description": "Chat session not found",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Failed to add prompt-response pair",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/api/refresh-token": {
            "post": {
                "description": "Handle refresh token validation, creation and creation of accessTokens too",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "refresh-token"
                ],
                "summary": "Handle refresh token validation, creation and creation of accessTokens too",
                "parameters": [
                    {
                        "description": "Refresh Token",
                        "name": "refreshToken",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "type": "string"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "refreshToken and accessToken",
                        "schema": {
                            "type": "object",
                            "additionalProperties": {
                                "type": "string"
                            }
                        }
                    },
                    "400": {
                        "description": "Invalid request payload",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "401": {
                        "description": "User not authenticated",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Failed to create refresh token",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/api/user-auth-token": {
            "get": {
                "description": "Fetch user info from auth token",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "auth-tokens"
                ],
                "summary": "Fetch user info from auth token",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "type": "string"
                            }
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "404": {
                        "description": "Not Found",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/api/user/{param}": {
            "get": {
                "description": "Fetch user info from id",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "user-from-email"
                ],
                "summary": "Fetch user info from id",
                "parameters": [
                    {
                        "type": "string",
                        "description": "ID of the user",
                        "name": "param",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "type": "string"
                            }
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "404": {
                        "description": "Not Found",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/chat": {
            "post": {
                "description": "Chat with the model",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "chat"
                ],
                "summary": "Chat with the model",
                "parameters": [
                    {
                        "description": "Chat request",
                        "name": "chat",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/server.chatRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "type": "string"
                            }
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "404": {
                        "description": "Not Found",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/status": {
            "get": {
                "description": "Status of the API",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "status"
                ],
                "summary": "Status of the API",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "type": "string"
                            }
                        }
                    },
                    "404": {
                        "description": "Not Found",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "agent.AgentInfo": {
            "type": "object",
            "properties": {
                "created_at": {
                    "type": "string"
                },
                "hostname": {
                    "type": "string"
                },
                "id": {
                    "type": "string"
                },
                "ip_address": {
                    "type": "string"
                },
                "kernel_version": {
                    "type": "string"
                },
                "os_version": {
                    "type": "string"
                },
                "user_id": {
                    "type": "string"
                }
            }
        },
        "chat.Chat": {
            "type": "object",
            "properties": {
                "agent_id": {
                    "type": "string"
                },
                "created_at": {
                    "type": "string"
                },
                "history": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/chat.PromptResponse"
                    }
                },
                "id": {
                    "type": "string"
                }
            }
        },
        "chat.PromptResponse": {
            "type": "object",
            "properties": {
                "prompt": {
                    "type": "string"
                },
                "response": {
                    "type": "string"
                },
                "type": {
                    "description": "\"commands\" or \"text\"",
                    "type": "string"
                }
            }
        },
        "server.chatRequest": {
            "type": "object",
            "properties": {
                "chat": {
                    "type": "string"
                },
                "history": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/server.content"
                    }
                }
            }
        },
        "server.content": {
            "type": "object",
            "properties": {
                "parts": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/server.part"
                    }
                },
                "role": {
                    "type": "string"
                }
            }
        },
        "server.part": {
            "type": "object",
            "properties": {
                "text": {
                    "type": "string"
                }
            }
        },
        "token.Token": {
            "type": "object",
            "properties": {
                "created_at": {
                    "type": "string"
                },
                "hashed_token": {
                    "type": "string"
                },
                "id": {
                    "type": "string"
                },
                "retrieved": {
                    "type": "boolean"
                },
                "token": {
                    "type": "string"
                },
                "user_id": {
                    "type": "string"
                }
            }
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "",
	Host:             "",
	BasePath:         "",
	Schemes:          []string{},
	Title:            "",
	Description:      "",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
