from parliament import Context, event
from langchain import PromptTemplate
from langchain.llms import OpenAI

import os
import sys

template = """
    Below is text that we received from person on the internet.
    Your goal is to:
    - Check the text if it is safe. It is OK if the text is not just in English
    - If not safe tell me that, and always tell what language it was as well.

    Below is the text:
    Text: {text}
    
"""

prompt = PromptTemplate(
    input_variables=["text"],
    template=template,
)


def get_api_key():
    # Make sure you have set the OPENAI_API_KEY env var
    return os.getenv('OPENAI_API_KEY')

def load_LLM(openai_api_key):
    llm = OpenAI(temperature=.7, openai_api_key=openai_api_key)
    return llm


@event
def main(context: Context):
    """
    Function template for LangChanin
    The context parameter contains the Flask request object and any
    CloudEvent received with the request.
    """

    # Make sure you have set the OPENAI_API_KEY env var
    llm = load_LLM(openai_api_key=get_api_key())

    # Parse the cloud Event Data, and populate the template
    event = context.cloud_event
    prompt_with_text = prompt.format(text=event.data)

    formatted_text = llm(prompt_with_text)
    context.cloud_event.data = formatted_text

    # The return value here will be applied as the data attribute
    # of a CloudEvent returned to the function invoker
    return context.cloud_event.data

