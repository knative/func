# Function Developer's Guide

Before learning to develop Functions, you should have the client [installed](installing_cli.md) and a configured provider.

## Local Setup and Configuration

Docker is required unless the --local flag is explicitly provided on creation
of a new function.

It is recommended to set your preferred image registry for publishing Functions
by setting the following environment variables:
```
export FUNCTION_REGISTRY=quay.io
export FUNCTION_NAMESPACE=alice
```
Alternately, when using the CLI these values can be provided using the --namespace and --registry 
flags.

## Examples

### Create a new Function:

```shell
> mkdir -p example.com/www
> cd example.com/www
> function create go
https://www.example.com
> curl https://www.example.com
OK
```

Update a Function:

After


### The Grid

Assuming you have followed one of the Getting Started guides, you are now connected to, and have created a new function on, a compatible set of compute resources.  From the perspective of your function, these resources are a utility; much like connecting to the electrical grid.  The basic units of this Compute Grid (henceforth called simply the Grid) is CPU RAM and Disk.  The functions you write will be able to run on any other variant of the infrastrucutre in much the same way as any electrical device can utilize the electrical grid from any provider, with certain caviats such as load and guarantees.

