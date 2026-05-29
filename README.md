# AWG-PORTAL

[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](https://opensource.org/licenses/MIT)
![GitHub last commit](https://img.shields.io/github/last-commit/DanilenkA/awg-portal/master)
[![Go Report Card](https://goreportcard.com/badge/github.com/DanilenkA/awg-portal)](https://goreportcard.com/report/github.com/DanilenkA/awg-portal)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/DanilenkA/awg-portal)

## Introduction

**AWG-PORTAL** — это форк [h44z/wg-portal](https://github.com/h44z/wg-portal) с полной поддержкой [AmneziaWGv2 ](https://github.com/amnezia-vpn/amneziawg-go) — протокола, устойчивого к DPI и блокировкам.

Портал предоставляет веб-интерфейс для управления VPN-серверами на базе **WireGuard** и **AmneziaWG**. Поддерживает создание/удаление пиров, генерацию конфигов, QR-коды, email-рассылку, мониторинг и REST API.

Обфускация AmneziaWG настраивается через веб-интерфейс для каждого интерфейса отдельно — параметры автоматически передаются в конфигурации клиентов.

## Возможности

* Полная поддержка **WireGuard** и **AmneziaWG** (amneziawg-go v2.x / awg)
* Обфускация трафика — защита от DPI (Deep Packet Inspection)
* Автовыбор AWG-параметров с возможностью ручной настройки
* Самодостаточный бинарник — всё в одном файле
* Адаптивный веб-интерфейс на Vue.js с тёмной темой и мультиязычностью
* Автовыбор IP из пула сети при создании пира
* QR-код для удобной настройки мобильных клиентов
* Отправка конфига по email
* Включение / отключение пиров без прерывания соединений
* Генерация wg-quick конфигов (`wgX.conf`)
* Аутентификация (БД, OAuth, LDAP), поддержка Passkey
* IPv6 готовность
* Docker-ready
* Работа с существующими WireGuard-интерфейсами
* Поддержка нескольких интерфейсов и бекендов (wgctl, MikroTik, pfSense)
* Управление маршрутизацией и DNS (как wg-quick)
* Prometheus-метрики для мониторинга
* REST API для управления и деплоя клиентов
* Webhook для кастомных действий

## Отличия от оригинала (h44z/wg-portal)

| Возможность | h44z/wg-portal | AWG-PORTAL |
|---|---|---|
| WireGuard | ✅ | ✅ |
| AmneziaWG (обфускация) | ❌ | ✅ |
| amneziawg-go | ❌ | ✅ (v2.x) |
| AWG-параметры в API | ❌ | ✅ |
| UAPI для AWG | ❌ | ✅ |
| AWG-бейдж в интерфейсе | ❌ | ✅ |
| SVG-логотип | ❌ | ✅ |

## Быстрый старт

```bash
# Скачать последний релиз
curl -LO https://github.com/DanilenkA/awg-portal/releases/latest/download/awg-portal-linux-amd64.tar.gz
tar xzf awg-portal-linux-amd64.tar.gz
sudo ./awg-portal --config config.yml
```

Пример конфига: [config.yml.sample](config.yml.sample)

Полная документация: [wgportal.org](https://wgportal.org) (оригинальная, функционал совместим).

## Установка AmneziaWG

AWG-PORTAL автоматически управляет процессом `amneziawg-go`. Если бинарный бандл содержит `amneziawg-go`, портал запустит его в фоне. В режиме `awg_mode: auto` портал сам определяет, какой протокол использовать, на основе настроек интерфейса.

Подробнее: [AmneziaWG](https://docs.amnezia.org/ru/documentation/amnezia-wg/)

## Сборка из исходников

```bash
# Требования: Go 1.23+, Node.js 20+
git clone git@github.com:DanilenkA/awg-portal.git
cd awg-portal

# Фронтенд
make frontend

# Бинарь
make build-amd64
```

## Application stack

* [amneziawg-go](https://github.com/amnezia-vpn/amneziawg-go) — AWG-протокол
* [wgctrl-go](https://github.com/WireGuard/wgctrl-go) и [netlink](https://github.com/vishvananda/netlink) — управление интерфейсами
* [Bootstrap](https://getbootstrap.com/) — HTML-шаблоны
* [Vue.js](https://vuejs.org/) — фронтенд

## Лицензия

MIT License. [MIT](LICENSE.txt)

## Благодарности

Огромное спасибо [h44z](https://github.com/h44z) за оригинальный [WireGuard Portal](https://github.com/h44z/wg-portal).
