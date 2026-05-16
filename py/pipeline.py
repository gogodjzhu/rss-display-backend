import argparse
import asyncio
import json
import logging
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "llm"))
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "crawl"))

from chat_openai_config import create_chat_openai_model
from rate_limiter import RateLimiter


logging.basicConfig(
    level=logging.INFO,
    format="[%(asctime)s] [pipeline] %(levelname)s %(message)s",
)
logger = logging.getLogger(__name__)


def load_input(path: str) -> dict:
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def save_output(path: str, data: dict) -> None:
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)


def run_filter_l1(data: dict) -> dict:
    llm = create_chat_openai_model(
        default_model=os.getenv("PIPELINE_LLM_MODEL", "qwen-plus"),
    )
    from langchain_core.prompts import ChatPromptTemplate
    from langchain_core.output_parsers import StrOutputParser

    device = data.get("device", {})
    items = data.get("items", [])

    if not items:
        return {"level1_ids": []}

    role = device.get("role", "")
    preference = device.get("preference", "")

    items_text = "\n".join(
        f"ID={item['id']}: {item.get('title', '')} ({item.get('url', '')})"
        for item in items
    )

    prompt = ChatPromptTemplate.from_messages([
        ("system", (
            "You are a content curator. Given a user's role and preference, "
            "select the most relevant article IDs from a list. "
            "Return ONLY a JSON array of integer IDs, no other text. "
            "Example: [1, 5, 12]"
        )),
        ("human", (
            "User role: {role}\n"
            "User preference: {preference}\n\n"
            "Articles:\n{items_text}\n\n"
            "Return the IDs of articles most relevant to this user as a JSON array."
        )),
    ])

    chain = prompt | llm | StrOutputParser()
    response = chain.invoke({
        "role": role,
        "preference": preference,
        "items_text": items_text,
    })

    response = response.strip()
    try:
        ids = json.loads(response)
    except json.JSONDecodeError:
        start = response.find("[")
        end = response.rfind("]") + 1
        if start >= 0 and end > start:
            ids = json.loads(response[start:end])
        else:
            logger.error("Failed to parse LLM response as JSON: %s", response)
            return {"level1_ids": []}

    return {"level1_ids": [int(i) for i in ids]}


async def run_crawl_async(data: dict, rate_limiter: RateLimiter) -> dict:
    from crawl4ai import AsyncWebCrawler, BrowserConfig, CrawlerRunConfig

    items = data.get("items", [])
    results = []

    if not items:
        return {"results": []}

    browser_config = BrowserConfig(headless=True)
    run_config = CrawlerRunConfig(word_count_threshold=1)

    async with AsyncWebCrawler(config=browser_config) as crawler:
        for item in items:
            item_id = item.get("id")
            url = item.get("url", "")

            if not url:
                results.append({
                    "id": item_id,
                    "content": "",
                    "success": False,
                    "error": "empty url",
                })
                continue

            domain = rate_limiter.extract_domain(url)
            await rate_limiter.acquire(domain)

            try:
                result = await crawler.arun(url=url, config=run_config)
                content = getattr(result, "markdown", "") or ""
                if content:
                    results.append({
                        "id": item_id,
                        "content": content,
                        "success": True,
                    })
                else:
                    results.append({
                        "id": item_id,
                        "content": "",
                        "success": False,
                        "error": "empty content",
                    })
            except Exception as e:
                logger.error("Crawl failed for item %d url=%s: %s", item_id, url, e)
                results.append({
                    "id": item_id,
                    "content": "",
                    "success": False,
                    "error": str(e),
                })

    return {"results": results}


def run_crawl(data: dict) -> dict:
    min_seconds = data.get("rate_limit_min_seconds", 30)
    max_seconds = data.get("rate_limit_max_seconds", 90)
    rate_limiter = RateLimiter(min_seconds=min_seconds, max_seconds=max_seconds)
    return asyncio.run(run_crawl_async(data, rate_limiter))


def run_summarize(data: dict) -> dict:
    llm = create_chat_openai_model(
        default_model=os.getenv("PIPELINE_LLM_MODEL", "qwen-plus"),
    )
    from langchain_core.prompts import ChatPromptTemplate
    from langchain_core.output_parsers import StrOutputParser

    items = data.get("items", [])
    results = []

    if not items:
        return {"results": []}

    for item in items:
        item_id = item.get("id")
        title = item.get("title", "")
        content = item.get("content", "")

        if not content:
            logger.warning("Skipping item %d: empty content", item_id)
            continue

        prompt = ChatPromptTemplate.from_messages([
            ("system", (
                "You are a content summarizer. Generate a concise abstract "
                "of approximately 200 characters in the same language as the article. "
                "Return ONLY the abstract text, nothing else."
            )),
            ("human", (
                "Title: {title}\n\nContent:\n{content}\n\n"
                "Generate a concise abstract (~200 chars) for this article."
            )),
        ])

        chain = prompt | llm | StrOutputParser()
        abstract = chain.invoke({"title": title, "content": content[:8000]})
        results.append({
            "id": item_id,
            "abstract": abstract.strip(),
        })

    return {"results": results}


def run_filter_l2(data: dict) -> dict:
    llm = create_chat_openai_model(
        default_model=os.getenv("PIPELINE_LLM_MODEL", "qwen-plus"),
    )
    from langchain_core.prompts import ChatPromptTemplate
    from langchain_core.output_parsers import StrOutputParser

    device = data.get("device", {})
    items = data.get("items", [])

    if not items:
        return {"level2_ids": []}

    role = device.get("role", "")
    preference = device.get("preference", "")

    items_text = "\n".join(
        f"ID={item['id']}: {item.get('title', '')}\n  Abstract: {item.get('abstract', '')}"
        for item in items
    )

    prompt = ChatPromptTemplate.from_messages([
        ("system", (
            "You are a content curator performing a second-round filter. "
            "Given a user's role, preference, and article summaries, "
            "select the MOST valuable articles for this user. "
            "Be more selective than the first round - choose only the best. "
            "Return ONLY a JSON array of integer IDs, no other text. "
            "Example: [5, 33]"
        )),
        ("human", (
            "User role: {role}\n"
            "User preference: {preference}\n\n"
            "Articles with summaries:\n{items_text}\n\n"
            "Return the IDs of the most valuable articles for this user as a JSON array."
        )),
    ])

    chain = prompt | llm | StrOutputParser()
    response = chain.invoke({
        "role": role,
        "preference": preference,
        "items_text": items_text,
    })

    response = response.strip()
    try:
        ids = json.loads(response)
    except json.JSONDecodeError:
        start = response.find("[")
        end = response.rfind("]") + 1
        if start >= 0 and end > start:
            ids = json.loads(response[start:end])
        else:
            logger.error("Failed to parse LLM response as JSON: %s", response)
            return {"level2_ids": []}

    return {"level2_ids": [int(i) for i in ids]}


def run_compose(data: dict) -> dict:
    llm = create_chat_openai_model(
        default_model=os.getenv("PIPELINE_LLM_MODEL", "qwen-plus"),
    )
    from langchain_core.prompts import ChatPromptTemplate
    from langchain_core.output_parsers import StrOutputParser

    device = data.get("device", {})
    items = data.get("items", [])

    role = device.get("role", "")
    preference = device.get("preference", "")

    items_text = "\n".join(
        f"- **[{item.get('title', '')}]({item.get('url', '')})**: {item.get('abstract', '')}"
        for item in items
    )

    prompt = ChatPromptTemplate.from_messages([
        ("system", (
            "You are a content curator generating a personalized reading report in Markdown. "
            "Organize the articles by topic/category based on the user's preference. "
            "Use the same language as the articles. "
            "Format each article as a bullet point with title as link and abstract as description. "
            "Include a brief introduction and conclusion tailored to the user's interests."
        )),
        ("human", (
            "User role: {role}\n"
            "User preference: {preference}\n\n"
            "Selected articles:\n{items_text}\n\n"
            "Generate a structured Markdown reading report."
        )),
    ])

    chain = prompt | llm | StrOutputParser()
    report = chain.invoke({
        "role": role,
        "preference": preference,
        "items_text": items_text,
    })

    return {"report": report.strip()}


def main():
    parser = argparse.ArgumentParser(description="RSS Pipeline")
    parser.add_argument("--mode", required=True, choices=[
        "filter_l1", "crawl", "summarize", "filter_l2", "compose"
    ])
    parser.add_argument("--input", required=True, help="Path to input JSON file")
    parser.add_argument("--output", required=True, help="Path to output JSON file")
    args = parser.parse_args()

    logger.info("Starting pipeline mode=%s", args.mode)
    data = load_input(args.input)

    handlers = {
        "filter_l1": run_filter_l1,
        "crawl": run_crawl,
        "summarize": run_summarize,
        "filter_l2": run_filter_l2,
        "compose": run_compose,
    }

    handler = handlers[args.mode]
    result = handler(data)

    save_output(args.output, result)
    logger.info("Pipeline mode=%s completed", args.mode)


if __name__ == "__main__":
    main()