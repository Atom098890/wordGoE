#!/bin/bash

# Define colors for better UI
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Functions for output messages
function print_message() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

function print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

function print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

function print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check requirements
function check_requirements() {
    print_message "Checking requirements..."
    
    # Check if Go is installed
    if ! command -v go &> /dev/null; then
        print_error "Go is not installed. Please install Go first."
        return 1
    fi
    
    print_success "All requirements are met!"
    return 0
}

# Setup environment
function setup_env() {
    print_message "Setting up environment..."
    
    # Check if .env file already exists
    if [ -f .env ]; then
        print_warning ".env file already exists. Do you want to overwrite it? (y/n)"
        read -r overwrite
        if [[ ! $overwrite =~ ^[Yy]$ ]]; then
            print_message "Keeping existing .env file."
            return 0
        fi
    fi
    
    # Get Telegram bot token
    print_message "Enter your Telegram bot token (obtained from @BotFather):"
    read -r telegram_token
    
    # Database path
    print_message "Enter path for SQLite database (leave empty for default ~/.engbot/database.db):"
    read -r db_path
    
    # OpenAI API key (optional)
    print_message "Enter your OpenAI API key (optional, for AI-powered features):"
    read -r openai_key
    
    # Create .env file
    cat > .env << EOF
# Telegram Bot Settings
TELEGRAM_BOT_TOKEN=$telegram_token

# Database Settings
DB_PATH=$db_path

# OpenAI API Settings (optional)
OPENAI_API_KEY=$openai_key
EOF
    
    print_success ".env file created successfully!"
    return 0
}

# Import words function
function import_words() {
    local file="$1"
    
    if [ -z "$file" ]; then
        print_error "No file specified for import."
        exit 1
    fi
    
    if [ ! -f "$file" ]; then
        print_error "File not found: $file."
        exit 1
    fi
    
    print_message "Importing words from $file..."
    
    # Run the import command
    go run main.go -import -file="$file"
    
    if [ $? -eq 0 ]; then
        print_success "Words successfully imported from $file."
    else
        print_error "Failed to import words from $file."
        exit 1
    fi
}

# Function to run the bot
function run_bot() {
    print_message "Запускаю бота..."
    go run main.go
}

# Show help information
function show_help() {
    print_message "===== English Words Telegram Bot ====="
    print_message "Использование: $0 [command] [options]"
    print_message ""
    print_message "Команды:"
    print_message "  setup       - Настроить окружение (создать .env файл)"
    print_message "  import      - Импортировать слова из файла"
    print_message "  run         - Запустить бота"
    print_message "  reset-db    - Сбросить базу данных"
    print_message "  help        - Показать эту справку"
    print_message ""
    print_message "Опции для импорта:"
    print_message "  --sheet <name>     - Имя листа Excel (по умолчанию Sheet1)"
    print_message "  --word-col <col>   - Колонка с английскими словами (по умолчанию A)"
    print_message "  --trans-col <col>  - Колонка с переводами (по умолчанию B)"
    print_message "  --context-col <col>- Колонка с контекстом (по умолчанию C)"
    print_message "  --topic-col <col>  - Колонка с темами (по умолчанию D)"
    print_message "  --diff-col <col>   - Колонка со сложностью (по умолчанию E)"
    print_message "  --pron-col <col>   - Колонка с произношением (по умолчанию F)"
    print_message ""
    print_message "Примеры:"
    print_message "  $0                              - Запустить полную настройку и запуск"
    print_message "  $0 setup                        - Только настроить окружение"
    print_message "  $0 import words.xlsx            - Импортировать слова из Excel-файла"
    print_message "  $0 import words.csv --sheet Sheet2 - Импортировать из CSV с параметрами"
    print_message "  $0 run                          - Только запустить бота"
    exit 0
}

# Function to reset the database
function reset_db() {
    print_message "Сброс базы данных..."
    
    # Check for DB_PATH in .env file
    if [ -f ".env" ]; then
        DB_PATH=$(grep -o 'DB_PATH=.*' .env | cut -d '=' -f2)
    fi
    
    # Use default path if not found
    if [ -z "$DB_PATH" ]; then
        DB_PATH="./data/engbot.db"
    fi
    
    # Check if the file exists
    if [ -f "$DB_PATH" ]; then
        print_warning "Найдена существующая база данных: $DB_PATH"
        read -p "Вы уверены, что хотите удалить её? Все данные будут потеряны! (y/n): " ANSWER
        if [ "$ANSWER" = "y" ] || [ "$ANSWER" = "Y" ]; then
            rm "$DB_PATH"
            print_success "База данных удалена. Она будет пересоздана при следующем запуске бота."
        else
            print_message "Операция отменена."
        fi
    else
        print_warning "База данных не найдена по пути: $DB_PATH"
    fi
}

# Main function
function main() {
    # If no arguments, run interactive mode
    if [ $# -eq 0 ]; then
        print_message "===== English Words Telegram Bot ====="
        
        # Check requirements
        check_requirements
        
        # Setup environment
        setup_env
        
        # Ask if user wants to reset the database
        read -p "Хотите сбросить базу данных? (y/n): " ANSWER
        if [ "$ANSWER" = "y" ] || [ "$ANSWER" = "Y" ]; then
            reset_db
        fi
        
        # Ask if user wants to import words
        read -p "Хотите импортировать слова из файла? (y/n): " ANSWER
        if [ "$ANSWER" = "y" ] || [ "$ANSWER" = "Y" ]; then
            read -p "Укажите путь к файлу: " FILE_PATH
            import_words "$FILE_PATH"
        fi
        
        # Ask if user wants to run the bot
        read -p "Хотите запустить бота сейчас? (y/n): " ANSWER
        if [ "$ANSWER" = "y" ] || [ "$ANSWER" = "Y" ]; then
            run_bot
        else
            print_message "Для запуска бота используйте команду: $0 run"
        fi
        return
    fi
    
    # Process commands
    case "$1" in
        setup)
            check_requirements
            setup_env
            ;;
        import)
            shift
            import_words "$@"
            ;;
        run)
            run_bot
            ;;
        reset-db)
            reset_db
            ;;
        help)
            show_help
            ;;
        *)
            print_error "Неизвестная команда: $1"
            print_message "Используйте '$0 help' для получения справки."
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@" 