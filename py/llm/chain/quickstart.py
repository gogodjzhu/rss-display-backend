import sys
from pathlib import Path

from langchain.agents import create_agent

sys.path.append(str(Path(__file__).resolve().parents[1]))

from chat_openai_config import create_chat_openai_model

def get_weather(city: str) -> str:
    """Get weather for a given city."""
    return f"It's always sunny in {city}!"

def main() -> None:
    model = create_chat_openai_model()
    agent = create_agent(
        model=model,
        tools=[get_weather],
        system_prompt="You are a helpful assistant",
    )

    result = agent.invoke(
        {"messages": [{"role": "user", "content": "What's the weather in San Francisco?"}]}
    )
    print(result["messages"][-1].content_blocks)


if __name__ == "__main__":
    main()