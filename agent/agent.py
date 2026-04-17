"""SpecGuard Agent - LangChain ReAct agent for API governance tasks.

This agent can:
1. Initialize and scan repositories for API specs
2. Diff snapshots to detect changes
3. Generate standards, risk, drift, and doc consistency reports
4. Read and interpret report artifacts
5. Provide protocol (REST vs gRPC) recommendations

Usage:
    from agent.agent import create_agent, run_agent

    agent = create_agent()
    result = run_agent(agent, "Scan the repo at /path/to/repo and generate a risk report")
"""

import os
from typing import Optional

from langchain_core.messages import HumanMessage, SystemMessage
from langchain_core.prompts import ChatPromptTemplate, MessagesPlaceholder

from agent.tools import get_all_tools

SYSTEM_PROMPT = """You are SpecGuard Agent, an AI-powered API governance assistant.

Your capabilities:
1. **Scan** repositories to discover and normalize OpenAPI and Protobuf specifications
2. **Diff** spec snapshots to detect breaking changes, additions, and mutations
3. **Report** on standards compliance, documentation consistency, risk scores, and protocol recommendations
4. **Read** generated report artifacts to provide insights and recommendations

When the user asks you to analyze a repository:
1. First check if it's already initialized (look for .specguard/config.yaml)
2. Run a scan to create snapshots
3. Generate appropriate reports based on what's available
4. Read and summarize the key findings

Always provide actionable recommendations. When reporting risk scores:
- CRITICAL (85-100): Immediate action required
- HIGH (60-84): Address before next release
- MEDIUM (40-59): Plan remediation
- LOW (20-39): Monitor
- INFO (0-19): No action needed

For protocol recommendations, explain the SOAF model signals that drove the decision.
"""


def create_agent(
    model_name: str = "gpt-4o",
    api_key: Optional[str] = None,
    temperature: float = 0.0,
):
    """Create a SpecGuard LangChain ReAct agent.

    Args:
        model_name: OpenAI model name (default: gpt-4o)
        api_key: OpenAI API key (falls back to OPENAI_API_KEY env var)
        temperature: LLM temperature (default: 0.0 for deterministic output)

    Returns:
        A LangChain agent executor ready to process queries.
    """
    try:
        from langchain_openai import ChatOpenAI
        from langchain.agents import AgentExecutor, create_tool_calling_agent
    except ImportError as e:
        raise ImportError(
            "Required packages not installed. Run: pip install -r agent/requirements.txt"
        ) from e

    key = api_key or os.environ.get("OPENAI_API_KEY")
    if not key:
        raise ValueError(
            "OpenAI API key required. Set OPENAI_API_KEY env var or pass api_key parameter."
        )

    llm = ChatOpenAI(
        model=model_name,
        api_key=key,
        temperature=temperature,
    )

    tools = get_all_tools()

    prompt = ChatPromptTemplate.from_messages([
        SystemMessage(content=SYSTEM_PROMPT),
        MessagesPlaceholder(variable_name="chat_history", optional=True),
        ("human", "{input}"),
        MessagesPlaceholder(variable_name="agent_scratchpad"),
    ])

    agent = create_tool_calling_agent(llm, tools, prompt)
    executor = AgentExecutor(
        agent=agent,
        tools=tools,
        verbose=True,
        max_iterations=15,
        handle_parsing_errors=True,
    )

    return executor


def run_agent(
    agent_executor,
    query: str,
    chat_history: Optional[list] = None,
) -> str:
    """Run the SpecGuard agent with a user query.

    Args:
        agent_executor: The agent executor from create_agent()
        query: User's natural language query
        chat_history: Optional conversation history

    Returns:
        The agent's response string.
    """
    inputs = {"input": query}
    if chat_history:
        inputs["chat_history"] = chat_history

    result = agent_executor.invoke(inputs)
    return result.get("output", "No output generated.")


if __name__ == "__main__":
    import sys

    if len(sys.argv) < 2:
        print("Usage: python -m agent.agent '<query>'")
        print("Example: python -m agent.agent 'Scan /home/user/myrepo and show risk report'")
        sys.exit(1)

    query = " ".join(sys.argv[1:])
    try:
        executor = create_agent()
        response = run_agent(executor, query)
        print(response)
    except ValueError as e:
        print(f"Error: {e}")
        sys.exit(1)
