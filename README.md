# Bsky Feed Generators Tracker

This repository contains the code for downloading feed posts and their metadata from the BlueSky social platform (via the [Indigo API](https://github.com/bluesky-social/indigo)) and storing them in a PostgreSQL database.

The code and dataset were used for the research paper **Looking AT the Blue Skies of Bluesky**,
which was presented at **IMC'24** ([link](https://dl.acm.org/doi/10.1145/3646547.3688407), [arXiv](https://arxiv.org/abs/2408.12449)).

If you use these tools or datasets for academic work, please cite our publication:

```
@inproceedings{10.1145/3646547.3688407,
author = {Balduf, Leonhard and Sokoto, Saidu and Ascigil, Onur and Tyson, Gareth and Scheuermann, Bj\"{o}rn and Korczy\'{n}ski, Maciej and Castro, Ignacio and Kr\'{o}l, Michaundefined},
title = {Looking AT the Blue Skies of Bluesky},
year = {2024},
isbn = {9798400705922},
publisher = {Association for Computing Machinery},
address = {New York, NY, USA},
url = {https://doi.org/10.1145/3646547.3688407},
doi = {10.1145/3646547.3688407},
abstract = {The pitfalls of centralized social networks, such as Facebook and Twitter/X, have led to concerns about control, transparency, and accountability. Decentralized social networks have emerged as a result with the goal of empowering users. These decentralized approaches come with their own trade-offs, and therefore multiple architectures exist. In this paper, we conduct the first large-scale analysis of Bluesky, a prominent decentralized microblogging platform. In contrast to alternative approaches (e.g. Mastodon), Bluesky decomposes and opens the key functions of the platform into subcomponents that can be provided by third party stakeholders. We collect a comprehensive dataset covering all the key elements of Bluesky, study user activity and assess the diversity of providers for each sub-components.},
booktitle = {Proceedings of the 2024 ACM on Internet Measurement Conference},
pages = {76–91},
numpages = {16},
keywords = {bluesky, decentralized social networks, social network analysis},
location = {Madrid, Spain},
series = {IMC '24}
}
```

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
