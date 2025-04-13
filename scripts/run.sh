#!/bin/bash

# This script provides various commands for managing the English Words Bot

# Check if a command is provided
if [ $# -lt 1 ]; then
    echo "Usage: $0 <command> [arguments...]"
    echo "Commands:"
    echo "  run             - Run the bot"
    echo "  build           - Build the bot"
    echo "  import <file>   - Import words from Excel file"
    echo "  setup-db        - Set up the database"
    echo "  help            - Show this help message"
    exit 1
fi

# Get the command
COMMAND=$1
shift

# Execute the appropriate action based on the command
case $COMMAND in
    run)
        # Run the bot
        echo "Starting the bot..."
        go run main.go
        ;;
        
    build)
        # Build the bot
        echo "Building the bot..."
        go build -o engbot main.go
        echo "Build complete. Use ./engbot to run the bot."
        ;;
        
    import)
        # Check if a file is provided
        if [ $# -lt 1 ]; then
            echo "Error: Excel file path is required for import."
            echo "Usage: $0 import <file>"
            exit 1
        fi
        
        FILE=$1
        
        # Check if the file exists
        if [ ! -f "$FILE" ]; then
            echo "Error: File does not exist: $FILE"
            exit 1
        fi
        
        # Run the import
        echo "Importing words from $FILE..."
        go run main.go -import -file "$FILE"
        ;;
        
    setup-db)
        # Set up the database
        echo "Setting up the database..."
        # Here you would typically run migrations using a tool like golang-migrate
        # For example:
        # migrate -path internal/database/migrations -database "postgres://username:password@localhost:5432/dbname?sslmode=disable" up
        
        echo "This command is not fully implemented yet. Please refer to documentation for database setup instructions."
        ;;
        
    help)
        # Show help message
        echo "English Words Bot - Commands:"
        echo "  run             - Run the bot"
        echo "  build           - Build the bot"
        echo "  import <file>   - Import words from Excel file"
        echo "  setup-db        - Set up the database"
        echo "  help            - Show this help message"
        ;;
        
    *)
        # Unknown command
        echo "Error: Unknown command: $COMMAND"
        echo "Use '$0 help' to see available commands."
        exit 1
        ;;
esac

exit 0 