# Knowledge Base Documentation

This is a sample knowledge base document that demonstrates how to upload text content to a Dify dataset using Terraform.

## Overview

Dify datasets allow you to organize and index documents for knowledge retrieval in your applications. This document will be chunked and indexed according to the dataset's chunking strategy.

## Key Features

- **Automatic Chunking**: Documents are automatically split into chunks based on the configured process rule
- **Vector Indexing**: Chunks are converted to vector embeddings for semantic search
- **Async Processing**: Document indexing happens asynchronously, with the Terraform resource polling for completion

## Usage

This document serves as an example of how text content can be uploaded directly to a dataset without requiring file upload.

## Benefits

- **Fast Upload**: Text content can be uploaded directly without file I/O
- **Simple Management**: Content is version-controlled alongside your infrastructure code
- **Easy Updates**: Changes to documentation are tracked in git
