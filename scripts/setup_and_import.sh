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
    print_message "Проверка необходимых программ..."
    
    # Check for go
    if ! command -v go &> /dev/null; then
        print_error "Go не установлен. Пожалуйста, установите Go для запуска бота."
        exit 1
    fi
    
    # Check for PostgreSQL
    if ! command -v psql &> /dev/null; then
        print_warning "PostgreSQL не установлен. Для работы бота требуется доступ к PostgreSQL."
    fi
    
    print_success "Все необходимые программы установлены."
}

# Setup environment
function setup_env() {
    print_message "Настройка окружения..."
    
    # Check if .env file exists
    if [ -f ".env" ]; then
        print_warning "Файл .env уже существует. Хотите пересоздать его? (y/n): "
        read -r ANSWER
        if [ "$ANSWER" != "y" ] && [ "$ANSWER" != "Y" ]; then
            print_message "Пропускаем создание .env файла."
            return
        fi
    fi
    
    print_message "Создание файла .env с настройками..."
    
    # Get Telegram bot token
    echo -n "Введите токен Telegram бота: "
    read -r BOT_TOKEN
    
    # Get database connection info
    echo -n "Введите имя пользователя PostgreSQL (по умолчанию postgres): "
    read -r DB_USER
    DB_USER=${DB_USER:-postgres}
    
    echo -n "Введите пароль PostgreSQL: "
    read -rs DB_PASSWORD
    echo ""
    
    echo -n "Введите имя базы данных (по умолчанию engbot): "
    read -r DB_NAME
    DB_NAME=${DB_NAME:-engbot}
    
    echo -n "Введите хост базы данных (по умолчанию localhost): "
    read -r DB_HOST
    DB_HOST=${DB_HOST:-localhost}
    
    echo -n "Введите порт базы данных (по умолчанию 5432): "
    read -r DB_PORT
    DB_PORT=${DB_PORT:-5432}
    
    # Optional OpenAI API key
    echo -n "Введите ключ API OpenAI (опционально): "
    read -r OPENAI_API_KEY
    
    # Write to .env file
    cat > .env << EOF
# Telegram bot token
TELEGRAM_BOT_TOKEN=$BOT_TOKEN

# Database connection
DB_USER=$DB_USER
DB_PASSWORD=$DB_PASSWORD
DB_NAME=$DB_NAME
DB_HOST=$DB_HOST
DB_PORT=$DB_PORT

# OpenAI API for advanced features (optional)
OPENAI_API_KEY=$OPENAI_API_KEY
EOF
    
    print_success "Файл .env успешно создан."
}

# Import words function
function import_words() {
    local file_path="$1"
    
    # Check if the file exists
    if [ ! -f "$file_path" ]; then
        print_error "File not found: $file_path"
        return 1
    fi
    
    # Check file extension to determine format
    local extension="${file_path##*.}"
    local format=""
    
    if [ "$extension" == "xlsx" ] || [ "$extension" == "xls" ]; then
        format="Excel"
    elif [ "$extension" == "csv" ]; then
        format="CSV"
    else
        print_error "Unsupported file format: .$extension (supported: xlsx, xls, csv)"
        return 1
    fi
    
    print_message "Importing words from $format file: $file_path..."
    
    # Run the import command
    if go run main.go -import -file "$file_path"; then
        print_success "Words imported successfully!"
        return 0
    else
        print_error "Failed to import words. Check the logs for details."
        return 1
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