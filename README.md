# gitchain
Decentralized Git Platform (Not for sale to Microsoft)


### Git UI
For the purpose of this hackathon we launch git server UI in Docker container. While there are multiple slim solutions ([alpine/git](https://hub.docker.com/r/alpine/git/) and [jkarlos/git-server-docker](https://hub.docker.com/r/jkarlos/git-server-docker/)), this project uses go-based [gitea](https://github.com/go-gitea/gitea) UI: [gitea/gitea](https://hub.docker.com/r/gitea/gitea/) with Maria DB.

To sturt the server you need to use [Docker](https://www.docker.com/) and [docker-compose](https://docs.docker.com/compose/).
Start web ui and database containers in detached mode:

```
docker-compose up -d
```

If you like to ssh to web or db instance, you could use `docker-compose exec web /bin/sh`.
Gitea provides swagger API: http://localhost:3000/api/swagger which allows quick exploration and interactive requests.

As the fist sted you need to login to Web UI http://localhost:3000 and create a user. That user would be an admin who could manage other users later.
