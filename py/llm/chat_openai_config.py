import os

from langchain_openai import ChatOpenAI


def resolve_chat_openai_settings(default_model: str = "deepseek-v4-flash", default_base_url: str | None = None) -> dict[str, str | None]:
    return {
        "api_key": os.getenv("OPENAI_API_KEY") or os.getenv("QWEN_API_KEY"),
        "base_url": os.getenv("OPENAI_BASE_URL") or os.getenv("QWEN_BASE_URL") or default_base_url,
        "model": os.getenv("OPENAI_MODEL", default_model),
    }


def create_chat_openai_model(default_model: str = "deepseek-v4-flash", default_base_url: str | None = None) -> ChatOpenAI:
    settings = resolve_chat_openai_settings(default_model=default_model, default_base_url=default_base_url)
    return ChatOpenAI(
        model=settings["model"],
        api_key=settings["api_key"],
        base_url=settings["base_url"],
    )