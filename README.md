# Bsky Feed Generators Tracker

This repository contains the code for downloading feed posts and their metadata from the BlueSky social platform (via the [Indigo API](https://github.com/bluesky-social/indigo)) and storing them in a PostgreSQL database.

## Prerequisites

Before running the code, you need to set up the following:

1. **Configuration File**:
    - Copy the `dummy_config.json` file and rename it to `config.json`. Then, fill in your Bluesky credentials in the new `config.json` file.
    - The file should contain the following fields:
      ```json
      {
        "username": "<your-bluesky-username>",
        "password": "<your-bluesky-password>"
      }
      ```
      
2. **DID List**:
    - Replace `test_did.csv` with a list of feed generators. The CSV file should contain rows with two columns:
      - **Column 1**: The DID (identifier) of the creator.
      - **Column 2**: The identifier for the feed.

3. **PostgreSQL Database**:
    - Set up a PostgreSQL database. You can use Docker to initialize it easily.
    - Run the following Docker command to set up PostgreSQL:
      ```bash
      docker run --name bsky-feed-db -e POSTGRES_PASSWORD=mysecretpassword -e POSTGRES_DB=feed-generators -p 5432:5432 -d postgres
      ```

    - Update the `connStr` in the `initDB()` function to match your database credentials. For example:
      ```go
      connStr := "user=username password=password dbname=feed-generators host=localhost port=5432 sslmode=disable"
      ```

## Database Schema

The following schema defines the structure of the database:

1. **feed_generators**: Stores metadata for feed generators (DIDs).
    ```sql
    CREATE TABLE feed_generators (
        aturi TEXT PRIMARY KEY,
        posts TEXT[] 
    );
    ```

2. **posts**: Stores the actual feed posts.
    ```sql
    CREATE TABLE posts (
        uri TEXT PRIMARY KEY,
        post_data JSONB 
    );
    ```

## Running the Code

1. **Install dependencies**:
   Make sure you have Go installed and then run the following to install the necessary dependencies:
   ```bash
   go mod tidy

2. **Start the Application**:

Once your database and configurations are set up, you can run the application:

go run main.go

The program will authenticate with BlueSky using the credentials provided in config.json, then it will begin fetching feed posts for the specified users listed in test_did.csv.

3.  **Logging**:

-  Logs will be stored in data/getfeeds-posts.log.

-  The metadata for feed generators and posts will be saved to your PostgreSQL database.

Database Structure

The following tables are used:

1.  feed_generators: Stores metadata for feed generators (DIDs).

-  aturi: The identifier for the feed generator.

-  metadata: JSONB field storing metadata for each feed generator.

2.  posts: Stores the actual feed posts.

-  uri: The unique identifier for each post.

-  post_data: JSONB field storing the post data.

## TODO
- [ ] Update to include the database schema details.

Contributing

If you'd like to contribute to this project, feel free to fork the repository and submit a pull request with your changes.
