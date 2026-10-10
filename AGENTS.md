# Neo development guidelines

The goal of this repo is to deliver a tool like Supabase CLI, but for the Neo4j graph database.
Like Supabase CLI, the neo tool supports the following:

- Provision a Docker container for running a local Neo4j, Community Edition
- Start and stop the container
- Create and maintain database migrations

Use Context7 to fetch documentation for Neo4j and Supabase CLI.
Refer to https://supabase.com/docs/guides/local-development if you are unable to find information about Supabase CLI on Context7.

The goal is to support the equivalent Supabase CLI commands for Neo:

## Setup Commands

### neo init

Like `supabase init`, this command creates a neo4j directory, which is safe to commit to version control. init creates local files only: it doesn't sign you in or connect the directory to a project on the Neo4j AuraDB cloud platform.

## Runtime Commands

### neo start

Like `supabase start`, it starts Neo4j services from the same directory. The first run takes time while the CLI downloads the Docker images.

### neo status

Like `supabase status` the CLI prints local credentials. See https://supabase.com/docs/guides/local-development/cli/getting-started#access-your-projects-services for a Supabase example.

### neo stop

Like `supabase stop` it stops running the stack.

## Migration Commands

For migration commands, refer to https://supabase.com/docs/guides/local-development/database-migrations if you are missing Supabase CLI information in Context7.

### neo migration new <filename>

Like `supabase migration new`, this will be used to create a new migration to store the DDL needed to create Neo4J nodes, relationships, labels and properties.

### neo db reset

Like `supabase db reset` this command is used to reset the database to the current migrations.

Seed data is loaded via `neo/seed.cypher`.

### neo db diff

This command, like `supabase db diff`, will diff changes.

## Implementation

Neo should be implemented as a CLI tool. As such, it should be easy to install the tool somewhere on the local file system, from where it can be executed.

The executable may require some extra files in order to run the Neo4j client, these should be co-located with the executable, e.g. within a `neo/lib` folder.

I'm thinking of three possible implementation platforms:

- Rust
- TypeScript
- Python

This order reflects my preference.
