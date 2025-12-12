#!/usr/bin/env python3
"""
FastAPI HTTP 服务
支持所有 HTTP 方法，打印请求的 method, uri, headers, body
"""

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse
from loguru import logger
import json
from typing import Any

app = FastAPI(title="HTTP Request Logger", version="1.0.0")


async def log_request(request: Request, body: Any = None) -> dict:
    """
    记录请求信息
    
    :param request: FastAPI Request 对象
    :param body: 请求体内容
    :return: 请求信息字典
    """
    # 获取请求方法
    method = request.method
    
    # 获取 URI（包含查询参数）
    uri = str(request.url)
    
    # 获取 headers
    headers = dict(request.headers)
    
    # 获取 body
    body_content = body
    if body_content is None:
        try:
            # 尝试读取原始 body
            body_bytes = await request.body()
            if body_bytes:
                try:
                    # 尝试解析为 JSON
                    body_content = json.loads(body_bytes.decode('utf-8'))
                except (json.JSONDecodeError, UnicodeDecodeError):
                    # 如果不是 JSON，则作为字符串
                    try:
                        body_content = body_bytes.decode('utf-8')
                    except UnicodeDecodeError:
                        body_content = body_bytes.hex()  # 二进制数据转为十六进制
        except Exception as e:
            logger.warning(f"读取请求体失败: {e}")
            body_content = None
    
    # 构建请求信息
    request_info = {
        "method": method,
        "uri": uri,
        "headers": headers,
        "body": body_content
    }
    
    # 打印请求信息
    logger.info("=" * 80)
    logger.info(f"Method: {method}")
    logger.info(f"URI: {uri}")
    logger.info(f"Headers: {json.dumps(headers, indent=2, ensure_ascii=False)}")
    logger.info(f"Body: {json.dumps(body_content, indent=2, ensure_ascii=False) if isinstance(body_content, (dict, list)) else body_content}")
    logger.info("=" * 80)
    
    return request_info


@app.api_route("/{path:path}", methods=["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "TRACE"])
async def catch_all(request: Request, path: str):
    """
    捕获所有 HTTP 方法和路径的请求
    
    :param request: FastAPI Request 对象
    :param path: 请求路径
    :return: JSON 响应
    """
    # 记录请求信息
    request_info = await log_request(request)
    
    # 返回请求信息
    return JSONResponse(
        content={
            "status": "success",
            "message": "Request received and logged",
            "request_info": request_info
        },
        status_code=200
    )


@app.get("/")
async def root():
    """
    根路径处理
    """
    return JSONResponse(
        content={
            "status": "success",
            "message": "HTTP Request Logger Service",
            "description": "This service logs all HTTP requests (method, URI, headers, body)",
            "endpoints": {
                "any_path": "Supports all HTTP methods (GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS, TRACE)"
            }
        }
    )


if __name__ == "__main__":
    import uvicorn
    
    logger.info("Starting HTTP Request Logger Service...")
    logger.info("Service will log all incoming requests (method, URI, headers, body)")
    logger.info("Access the service at http://localhost:8000")
    
    uvicorn.run(
        app,
        host="0.0.0.0",
        port=8000,
        log_level="info"
    )

