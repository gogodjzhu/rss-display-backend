import asyncio
import json
import os
import sys

from crawl4ai import AsyncWebCrawler, BrowserConfig, CacheMode, CrawlerRunConfig, LLMConfig
from crawl4ai import LLMExtractionStrategy
from pydantic import BaseModel, Field


class PageInfo(BaseModel):
    title: str = Field(..., description="The page title or article title.")
    published_time: str = Field(
        ..., description="The page publish time in the source text. Use an empty string if missing."
    )
    content: str = Field(..., description="The main body content of the page without navigation or ads.")


async def extract_page_info(url: str) -> str:
    api_key = os.getenv("DASHSCOPE_API_KEY")
    if not api_key:
        raise RuntimeError("DASHSCOPE_API_KEY is not set")

    browser_config = BrowserConfig(verbose=True)
    llm_config = LLMConfig(
        provider="dashscope/qwen-plus",
        api_token=api_key,
        base_url="https://dashscope.aliyuncs.com/compatible-mode/v1",
    )

    run_config = CrawlerRunConfig(
        word_count_threshold=1,
        extraction_strategy=LLMExtractionStrategy(
            llm_config=llm_config,
            schema=PageInfo.model_json_schema(),
            extraction_type="schema",
            instruction=(
                "Extract the main page information from the crawled content. "
                "Return exactly these fields: title, published_time, content. "
                "title should be the article or page title. "
                "published_time should be the publication time found on the page; use an empty string if the page does not provide one. "
                "content should contain the main article/document body only, excluding navigation, footer, ads, recommendations, and unrelated boilerplate."
            ),
        ),
        cache_mode=CacheMode.BYPASS,
    )

    async with AsyncWebCrawler(config=browser_config) as crawler:
        result = await crawler.arun(url=url, config=run_config)
        return result.extracted_content


async def main() -> None:
    if len(sys.argv) < 2:
        raise SystemExit("Usage: python py/crawl/demo/page_info_extracted.py <url>")

    extracted_content = await extract_page_info(sys.argv[1])

    try:
        parsed = json.loads(extracted_content)
        print(json.dumps(parsed, ensure_ascii=False, indent=2))
    except json.JSONDecodeError:
        print(extracted_content)


if __name__ == "__main__":
    asyncio.run(main())