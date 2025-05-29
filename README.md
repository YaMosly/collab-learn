# 🚀 CollabLearn

<div align="center">
  <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version">
  <img src="https://img.shields.io/badge/PostgreSQL-15+-4169E1?style=for-the-badge&logo=postgresql&logoColor=white" alt="PostgreSQL">
  <img src="https://img.shields.io/badge/Redis-7+-DC382D?style=for-the-badge&logo=redis&logoColor=white" alt="Redis">
  <img src="https://img.shields.io/badge/Docker-20.10+-2496ED?style=for-the-badge&logo=docker&logoColor=white" alt="Docker">
  <img src="https://img.shields.io/badge/WebSockets-Supported-010101?style=for-the-badge&logo=socket.io&logoColor=white" alt="WebSockets">
</div>

<br>

<div align="center">
  <h3>🎓 Платформа для совместного изучения веб-разработки в реальном времени</h3>
  <p>Изучайте HTML/CSS вместе с общим редактором кода и мгновенным предпросмотром</p>
</div>

## ✨ Возможности

- 🔄 **Совместная работа в реальном времени** - Все изменения кода мгновенно синхронизируются между всеми подключенными пользователями
- 👥 **Поддержка нескольких пользователей** - Видите, сколько пользователей подключено к вашей доске
- 🎨 **Красивый редактор кода** - Подсветка синтаксиса с темой Material Ocean
- 👁️ **Предпросмотр в реальном времени** - Мгновенно видите результат изменений HTML/CSS
- 🔗 **Простой обмен** - Делитесь досками с помощью простой ссылки
- 💾 **Постоянное хранение** - Все доски сохраняются автоматически
- 🌐 **WebSocket-соединение** - Обновления в реальном времени с низкой задержкой
- 🐳 **Докеризировано** - Простое развертывание с Docker Compose

## 🛠️ Технологический стек

### Backend
- **Go (Golang)** - Высокопроизводительный серверный бэкенд
- **Chi Router** - Легковесный HTTP-роутер
- **Gorilla WebSocket** - Двунаправленная связь в реальном времени
- **PostgreSQL** - Надежное хранение данных
- **Redis** - Pub/Sub для синхронизации в реальном времени и кеширования

### Frontend
- **CodeMirror 5** - Профессиональный редактор кода с подсветкой синтаксиса
- **Vanilla JavaScript** - Без зависимостей от фреймворков
- **Modern CSS** - Кастомные свойства, анимации и градиенты

## 🚀 Быстрый старт

### Требования
- Docker & Docker Compose
- Git

### Установка

1. **Клонируйте репозиторий**
```bash
git clone https://github.com/yourusername/collab-learn.git
cd collab-learn
```

2. **Запустите с помощью Docker Compose**
```bash
docker-compose up --build
```

3. **Откройте в браузере**
```
http://localhost:8080
```

Вот и всё! 🎉

## 🏗️ Архитектура

```
collab-learn/
├── cmd/server/         # Точка входа приложения
├── internal/           # Приватный код приложения
│   ├── database/       # Подключение к PostgreSQL и миграции
│   ├── handlers/       # Обработчики HTTP-запросов
│   ├── models/         # Модели данных
│   ├── redis/          # Redis-клиент и pub/sub
│   └── websocket/      # WebSocket-хаб и управление клиентами
├── web/static/         # Статические файлы фронтенда
│   ├── css/           # Стили
│   ├── js/            # Клиентский JavaScript
│   └── index.html     # Основной HTML-файл
├── migrations/         # Миграции базы данных
├── docker-compose.yml  # Конфигурация Docker-сервисов
├── Dockerfile         # Контейнер приложения
└── go.mod            # Go-зависимости
```

## 🔧 Конфигурация

Переменные окружения (задаются в docker-compose.yml):

| Переменная | По умолчанию | Описание |
|----------|---------|-------------|
| `DB_HOST` | postgres | Хост PostgreSQL |
| `DB_PORT` | 5432 | Порт PostgreSQL |
| `DB_USER` | collablearn | Пользователь БД |
| `DB_PASSWORD` | collablearn123 | Пароль БД |
| `DB_NAME` | collablearn | Имя БД |
| `REDIS_HOST` | redis | Хост Redis |
| `REDIS_PORT` | 6379 | Порт Redis |
| `PORT` | 8080 | Порт приложения |

## 📡 API-эндпоинты

| Метод | Эндпоинт | Описание |
|--------|----------|-------------|
| `POST` | `/api/boards` | Создать новую доску |
| `GET` | `/api/boards` | Список всех досок |
| `GET` | `/api/boards/:id` | Получить детали доски |
| `PUT` | `/api/boards/:id` | Обновить код доски |
| `WS` | `/api/boards/:id/ws` | WebSocket-соединение |

## 🎮 Использование

1. **Создайте новую доску** - Нажмите кнопку "New Board"
2. **Поделитесь доской** - Нажмите "Share", чтобы скопировать URL
3. **Пишите код** - Используйте редакторы HTML и CSS
4. **Смотрите предпросмотр** - Изменения мгновенно появляются в окне предпросмотра
4. **Сотрудничайте** - Поделитесь URL с другими для совместного программирования

### Горячие клавиши

- `Ctrl/Cmd + Space` - Автодополнение
- `Ctrl/Cmd + /` - Переключить комментарий
- `Tab` - Отступ выделения или вставка 2 пробелов

## 🐳 Docker-сервисы

Приложение запускает три сервиса:

1. **app** - Go-сервер бэкенда
2. **postgres** - База данных PostgreSQL
3. **redis** - Redis для pub/sub и кеширования

## 🔐 Функции безопасности

- Санитизация HTML-контента в iframe
- Настройка CORS
- Защита от SQL-инъекций с помощью подготовленных запросов
- Проверка источника WebSocket

## 🚦 Разработка

### Локальная разработка

1. **Установите Go 1.21+**
2. **Установите зависимости**
```bash
go mod download
```

3. **Запустите локально**
```bash
go run cmd/server/main.go
```

### Сборка из исходников

```bash
CGO_ENABLED=0 go build -o collablearn cmd/server/main.go
```

## 📊 Производительность

- WebSocket-соединения обрабатывают тысячи одновременных пользователей
- Redis pub/sub обеспечивает доставку сообщений между экземплярами сервера
- Индексы PostgreSQL оптимизируют запросы к доскам
- Дебаунсинг на клиенте предотвращает избыточные обновления

## 🤝 Вклад в проект

1. Сделайте форк репозитория
2. Создайте ветку для вашей функции (`git checkout -b feature/amazing-feature`)
3. Зафиксируйте изменения (`git commit -m 'Add some amazing feature'`)
4. Отправьте изменения в ветку (`git push origin feature/amazing-feature`)
5. Откройте Pull Request

## 📝 Лицензия

Этот проект лицензирован под лицензией MIT - подробности см. в файле [LICENSE](LICENSE).

## 🙏 Благодарности

- [CodeMirror](https://codemirror.net/) за потрясающий редактор кода
- [Material Ocean](https://github.com/material-ocean) за красивую тему подсветки синтаксиса
- [Chi](https://github.com/go-chi/chi) за элегантный HTTP-роутер

---

<div align="center">
  Сделано с ❤️ для совместного обучения
</div>