# Agent

## Описание
Сбор системных метрик и отправка транзакций Heartbeat в chain-core. Пуш метрик в InfluxDB.

## Настройка
Редактируйте `config.yaml`, указывая `chain_rpc`, `node_id`, `pub_key`, `heartbeat_interval`, и параметры InfluxDB.

## Сборка и запуск
```bash
cd agent
docker build -t agent:latest .
# Если нужен внешний конфиг, монтируйте config.yaml
docker run --network <your-net> agent:latest