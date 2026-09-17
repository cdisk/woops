# 演示登录代理。只用 Python 标准库，不装任何依赖。
FROM python:3.12-alpine
WORKDIR /app
COPY app.py /app/app.py
EXPOSE 8088
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8088/healthz || exit 1
CMD ["python", "-u", "app.py"]
