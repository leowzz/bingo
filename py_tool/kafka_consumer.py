#!/usr/bin/env python3
"""
Kafka 消费者
从 Kafka 主题消费消息并打印
"""

import json
import signal
import sys
from typing import Optional

from kafka import KafkaConsumer
from loguru import logger


class KafkaMessageConsumer:
    """Kafka 消息消费者"""

    def __init__(
        self,
        brokers: list[str],
        topics: list[str],
        group_id: Optional[str] = None,
        auto_offset_reset: str = "latest",
    ):
        """
        初始化 Kafka 消费者

        :param brokers: Kafka broker 地址列表，例如: ["localhost:9092"]
        :param topics: 要消费的主题列表，例如: ["test-topic"]
        :param group_id: 消费者组 ID，如果为 None 则不使用消费者组
        :param auto_offset_reset: 偏移量重置策略，"earliest" 或 "latest"
        """
        self.brokers = brokers
        self.topics = topics
        self.group_id = group_id
        self.auto_offset_reset = auto_offset_reset
        self.consumer: Optional[KafkaConsumer] = None
        self.running = False

    def start(self):
        """启动消费者"""
        try:
            # 创建消费者配置
            consumer_config = {
                "bootstrap_servers": self.brokers,
                "auto_offset_reset": self.auto_offset_reset,
                "enable_auto_commit": True,
                "value_deserializer": lambda m: m.decode("utf-8") if m else None,
                "key_deserializer": lambda m: m.decode("utf-8") if m else None,
            }

            # 如果指定了消费者组，添加到配置
            if self.group_id:
                consumer_config["group_id"] = self.group_id

            # 创建消费者
            self.consumer = KafkaConsumer(*self.topics, **consumer_config)

            logger.info(f"Kafka 消费者已启动")
            logger.info(f"Brokers: {', '.join(self.brokers)}")
            logger.info(f"Topics: {', '.join(self.topics)}")
            if self.group_id:
                logger.info(f"Consumer Group: {self.group_id}")
            logger.info(f"Auto Offset Reset: {self.auto_offset_reset}")
            logger.info("=" * 80)

            self.running = True

            # 消费消息
            for message in self.consumer:
                if not self.running:
                    break

                self.handle_message(message)

        except KeyboardInterrupt:
            logger.info("收到中断信号，正在停止消费者...")
        except Exception as e:
            logger.error(f"消费消息时发生错误: {e}")
            raise
        finally:
            self.stop()

    def handle_message(self, message):
        """
        处理收到的消息

        :param message: Kafka 消息对象
        """
        try:
            # 提取消息信息
            topic = message.topic
            partition = message.partition
            offset = message.offset
            key = message.key
            value = message.value
            timestamp = message.timestamp

            # 打印消息信息
            logger.info("=" * 80)
            logger.info(f"Topic: {topic}")
            logger.info(f"Partition: {partition}")
            logger.info(f"Offset: {offset}")
            if timestamp:
                logger.info(f"Timestamp: {timestamp}")
            if key:
                logger.info(f"Key: {key}")

            # 尝试解析 JSON
            if value:
                try:
                    json_value = json.loads(value)
                    logger.info(f"Value (JSON):")
                    logger.info(json.dumps(json_value, indent=2, ensure_ascii=False))
                except (json.JSONDecodeError, TypeError):
                    logger.info(f"Value (Text): {value}")
            else:
                logger.info("Value: (empty)")

            logger.info("=" * 80)

        except Exception as e:
            logger.error(f"处理消息时发生错误: {e}")

    def stop(self):
        """停止消费者"""
        self.running = False
        if self.consumer:
            self.consumer.close()
            logger.info("Kafka 消费者已停止")


def main():
    """主函数"""
    import argparse

    parser = argparse.ArgumentParser(description="Kafka 消息消费者")
    parser.add_argument(
        "--brokers",
        nargs="+",
        default=["localhost:9092"],
        help="Kafka broker 地址列表（默认: localhost:9092）",
    )
    parser.add_argument(
        "--topics",
        nargs="+",
        required=True,
        help="要消费的主题列表（必需）",
    )
    parser.add_argument(
        "--group-id",
        type=str,
        default=None,
        help="消费者组 ID（可选）",
    )
    parser.add_argument(
        "--offset",
        type=str,
        choices=["earliest", "latest"],
        default="latest",
        help="偏移量重置策略（默认: latest）",
    )

    args = parser.parse_args()

    # 创建消费者
    consumer = KafkaMessageConsumer(
        brokers=args.brokers,
        topics=args.topics,
        group_id=args.group_id,
        auto_offset_reset=args.offset,
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

