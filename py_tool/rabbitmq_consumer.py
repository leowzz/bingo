#!/usr/bin/env python3
"""
RabbitMQ 消费者
从 RabbitMQ 队列消费消息并打印
"""

import json
import signal
import sys
from typing import Optional

import pika
from loguru import logger


class RabbitMQMessageConsumer:
    """RabbitMQ 消息消费者"""

    def __init__(
        self,
        host: str = "localhost",
        port: int = 5672,
        username: Optional[str] = None,
        password: Optional[str] = None,
        virtual_host: str = "/",
        queue: str = "test_queue",
        exchange: Optional[str] = None,
        routing_key: Optional[str] = None,
        exchange_type: str = "direct",
    ):
        """
        初始化 RabbitMQ 消费者

        :param host: RabbitMQ 服务器地址
        :param port: RabbitMQ 端口
        :param username: 用户名（可选）
        :param password: 密码（可选）
        :param virtual_host: 虚拟主机
        :param queue: 队列名称
        :param exchange: 交换机名称（可选，如果指定则绑定到交换机）
        :param routing_key: 路由键（可选，用于绑定到交换机）
        :param exchange_type: 交换机类型（direct, topic, fanout, headers）
        """
        self.host = host
        self.port = port
        self.username = username
        self.password = password
        self.virtual_host = virtual_host
        self.queue = queue
        self.exchange = exchange
        self.routing_key = routing_key or queue
        self.exchange_type = exchange_type

        self.connection: Optional[pika.BlockingConnection] = None
        self.channel: Optional[pika.channel.Channel] = None
        self.running = False

    def connect(self):
        """连接到 RabbitMQ"""
        try:
            # 构建连接参数
            credentials = None
            if self.username and self.password:
                credentials = pika.PlainCredentials(self.username, self.password)

            parameters = pika.ConnectionParameters(
                host=self.host,
                port=self.port,
                virtual_host=self.virtual_host,
                credentials=credentials,
            )

            # 建立连接
            self.connection = pika.BlockingConnection(parameters)
            self.channel = self.connection.channel()

            # 如果指定了交换机，声明并绑定
            if self.exchange:
                self.channel.exchange_declare(
                    exchange=self.exchange, exchange_type=self.exchange_type, durable=True
                )
                # 声明队列
                self.channel.queue_declare(queue=self.queue, durable=True)
                # 绑定队列到交换机
                self.channel.queue_bind(
                    exchange=self.exchange, queue=self.queue, routing_key=self.routing_key
                )
            else:
                # 直接声明队列
                self.channel.queue_declare(queue=self.queue, durable=True)

            logger.info(f"已连接到 RabbitMQ: {self.host}:{self.port}")
            logger.info(f"Virtual Host: {self.virtual_host}")
            logger.info(f"Queue: {self.queue}")
            if self.exchange:
                logger.info(f"Exchange: {self.exchange}")
                logger.info(f"Routing Key: {self.routing_key}")
            logger.info("=" * 80)

        except Exception as e:
            logger.error(f"连接 RabbitMQ 失败: {e}")
            raise

    def start(self):
        """启动消费者"""
        try:
            self.connect()
            self.running = True

            # 设置 QoS（每次只处理一条消息）
            self.channel.basic_qos(prefetch_count=1)

            # 开始消费消息
            self.channel.basic_consume(
                queue=self.queue, on_message_callback=self.handle_message, auto_ack=False
            )

            logger.info(f"开始消费队列: {self.queue}")
            logger.info("等待消息... (按 Ctrl+C 停止)")
            logger.info("=" * 80)

            # 开始消费
            self.channel.start_consuming()

        except KeyboardInterrupt:
            logger.info("收到中断信号，正在停止消费者...")
        except Exception as e:
            logger.error(f"消费消息时发生错误: {e}")
            raise
        finally:
            self.stop()

    def handle_message(
        self, ch: pika.channel.Channel, method: pika.spec.Basic.Deliver, properties: pika.spec.BasicProperties, body: bytes
    ):
        """
        处理收到的消息

        :param ch: 通道对象
        :param method: 方法对象，包含 delivery_tag, exchange, routing_key 等信息
        :param properties: 消息属性
        :param body: 消息体（字节）
        """
        try:
            # 提取消息信息
            exchange = method.exchange
            routing_key = method.routing_key
            delivery_tag = method.delivery_tag
            redelivered = method.redelivered

            # 打印消息信息
            logger.info("=" * 80)
            logger.info(f"Exchange: {exchange}")
            logger.info(f"Routing Key: {routing_key}")
            logger.info(f"Delivery Tag: {delivery_tag}")
            logger.info(f"Redelivered: {redelivered}")

            # 打印消息属性
            if properties:
                logger.info(f"Content Type: {properties.content_type}")
                logger.info(f"Content Encoding: {properties.content_encoding}")
                logger.info(f"Message ID: {properties.message_id}")
                logger.info(f"Correlation ID: {properties.correlation_id}")
                logger.info(f"Timestamp: {properties.timestamp}")
                if properties.headers:
                    logger.info(f"Headers: {properties.headers}")

            # 解析消息体
            if body:
                try:
                    # 尝试解析为 JSON
                    json_value = json.loads(body.decode("utf-8"))
                    logger.info(f"Body (JSON):")
                    logger.info(json.dumps(json_value, indent=2, ensure_ascii=False))
                except (json.JSONDecodeError, UnicodeDecodeError):
                    # 如果不是 JSON，作为文本处理
                    try:
                        text_value = body.decode("utf-8")
                        logger.info(f"Body (Text): {text_value}")
                    except UnicodeDecodeError:
                        # 如果无法解码为 UTF-8，显示十六进制
                        logger.info(f"Body (Binary): {body.hex()}")
            else:
                logger.info("Body: (empty)")

            logger.info("=" * 80)

            # 确认消息已处理
            ch.basic_ack(delivery_tag=delivery_tag)

        except Exception as e:
            logger.error(f"处理消息时发生错误: {e}")
            # 拒绝消息并重新入队
            ch.basic_nack(delivery_tag=method.delivery_tag, requeue=True)

    def stop(self):
        """停止消费者"""
        self.running = False
        if self.channel and not self.channel.is_closed:
            self.channel.stop_consuming()
        if self.connection and not self.connection.is_closed:
            self.connection.close()
        logger.info("RabbitMQ 消费者已停止")


def main():
    """主函数"""
    import argparse

    parser = argparse.ArgumentParser(description="RabbitMQ 消息消费者")
    parser.add_argument("--host", type=str, default="localhost", help="RabbitMQ 服务器地址（默认: localhost）")
    parser.add_argument("--port", type=int, default=5672, help="RabbitMQ 端口（默认: 5672）")
    parser.add_argument("--username", type=str, default=None, help="用户名（可选）")
    parser.add_argument("--password", type=str, default=None, help="密码（可选）")
    parser.add_argument("--virtual-host", type=str, default="/", help="虚拟主机（默认: /）")
    parser.add_argument("--queue", type=str, required=True, help="队列名称（必需）")
    parser.add_argument("--exchange", type=str, default=None, help="交换机名称（可选）")
    parser.add_argument("--routing-key", type=str, default=None, help="路由键（可选）")
    parser.add_argument(
        "--exchange-type",
        type=str,
        choices=["direct", "topic", "fanout", "headers"],
        default="direct",
        help="交换机类型（默认: direct）",
    )

    args = parser.parse_args()

    # 创建消费者
    consumer = RabbitMQMessageConsumer(
        host=args.host,
        port=args.port,
        username=args.username,
        password=args.password,
        virtual_host=args.virtual_host,
        queue=args.queue,
        exchange=args.exchange,
        routing_key=args.routing_key,
        exchange_type=args.exchange_type,
    )

    # 注册信号处理
    def signal_handler(sig, frame):
        logger.info("收到停止信号")
        consumer.stop()
        sys.exit(0)

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    # 启动消费者
    try:
        consumer.start()
    except Exception as e:
        logger.error(f"启动消费者失败: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()

