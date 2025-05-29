# CollabLearn

<div align="center">
  <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version">
  <img src="https://img.shields.io/badge/PostgreSQL-15+-4169E1?style=for-the-badge&logo=postgresql&logoColor=white" alt="PostgreSQL">
  <img src="https://img.shields.io/badge/Redis-7+-DC382D?style=for-the-badge&logo=redis&logoColor=white" alt="Redis">
  <img src="https://img.shields.io/badge/Docker-20.10+-2496ED?style=for-the-badge&logo=docker&logoColor=white" alt="Docker">
  <img src="https://img.shields.io/badge/WebSockets-Supported-010101?style=for-the-badge&logo=socket.io&logoColor=white" alt="WebSockets">
</div>

<br>

<div align="center">
  <h3> Платформа для совместного изучения веб-разработки в реальном времени</h3>
  <p>Изучайте HTML/CSS вместе с общим редактором кода и мгновенным предпросмотром</p>
</div>

## Возможности
![изображение](https://github.com/user-attachments/assets/3b10392f-1f67-4730-b8c2-8fc298c391ad)

-  **Совместная работа в реальном времени** - Все изменения кода мгновенно синхронизируются между всеми подключенными пользователями
-  **Поддержка нескольких пользователей** - Видите, сколько пользователей подключено к вашей доске
-  **Красивый редактор кода** - Подсветка синтаксиса с темой Material Ocean
-  **Предпросмотр в реальном времени** - Мгновенно видите результат изменений HTML/CSS
-  **Простой обмен** - Делитесь досками с помощью простой ссылки
-  **Постоянное хранение** - Все доски сохраняются автоматически
-  **WebSocket-соединение** - Обновления в реальном времени с низкой задержкой
-  **Докеризировано** - Простое развертывание с Docker Compose

## Технологический стек

### Backend
- **Go** - Бэкенд
- **Chi Router** - Легковесный HTTP-роутер
- **Gorilla WebSocket** - Двунаправленная связь в реальном времени
- **PostgreSQL** - Надежное хранение данных
- **Redis** - Pub/Sub для синхронизации в реальном времени и кеширования

### Frontend
- **CodeMirror 5** - Редактор кода с подсветкой синтаксиса
- **Vanilla JavaScript** - Без зависимостей от фреймворков
- **Modern CSS** - Кастомные свойства, анимации и градиенты

## Быстрый старт

### Требования
- Docker & Docker Compose
- Git

### Установка

1. **Клонируйте репозиторий**
```bash
git clone https://github.com/YaMosli/collab-learn.git
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

## Конфигурация

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

## API-эндпоинты

| Метод | Эндпоинт | Описание |
|--------|----------|-------------|
| `POST` | `/api/boards` | Создать новую доску |
| `GET` | `/api/boards` | Список всех досок |
| `GET` | `/api/boards/:id` | Получить детали доски |
| `PUT` | `/api/boards/:id` | Обновить код доски |
| `WS` | `/api/boards/:id/ws` | WebSocket-соединение |

## Использование
![изображение](https://github.com/user-attachments/assets/0e5fb5be-e70c-41a9-97be-55d7ca42d73b)

1. **Создайте новую доску** - Нажмите кнопку "New Board"
2. **Поделитесь доской** - Нажмите "Share", чтобы скопировать URL
3. **Пишите код** - Используйте редакторы HTML и CSS
4. **Смотрите предпросмотр** - Изменения мгновенно появляются в окне предпросмотра
4. **Сотрудничайте** - Поделитесь URL с другими для совместного программирования

### Горячие клавиши

- `Ctrl/Cmd + Space` - Автодополнение
- `Ctrl/Cmd + /` - Переключить комментарий
- `Tab` - Отступ выделения или вставка 2 пробелов

## Docker-сервисы

Приложение запускает три сервиса:

1. **app** - Go-сервер бэкенда
2. **postgres** - База данных PostgreSQL
3. **redis** - Redis для pub/sub и кеширования

---
