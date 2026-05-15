import sys
from pathlib import Path

from deepagents import create_deep_agent

sys.path.append(str(Path(__file__).resolve().parents[1]))

from chat_openai_config import create_chat_openai_model

def main() -> None:
    model = create_chat_openai_model(
        default_base_url="https://dashscope.aliyuncs.com/compatible-mode/v1",
    )
    agent = create_deep_agent(model=model)
    result = agent.invoke({"messages": [{"role": "user", "content": "给我推荐几部好看的电影吧！"}]})
    print(result["messages"][-1].content)


if __name__ == "__main__":
    main()