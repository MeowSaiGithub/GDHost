# GDHost
The **GDHost** is a middleware service that will allow user
to upload and host application using docker.
This project is part of my portfolio so I did not include any user management and authorization.

### Current Features

1. Manage Deployments
2. Upload/Create/Download *Dockerfile*
3. Manage Container (Create/Stop/Start/Delete/Log)

Please refer ***example_configuration.json*** for configuration.

Please refer to ***deployment.postman_collection.json*** for API documentation.

### How to use
1. Install docker on the server/machine
2. Install mongodb on the server/machine
3. Edit ***example_configuration.json*** to ***configuration.json***. Env variables will work the same.
4. Run the service as executable file. Need /internal/template folder to be in the same folder as executable.
5. Use the REST API to manage.

### How to run application
1. Archive the application into a zip file. Please do not include .git or hidden files.
2. Upload into the server.
3. Either upload or generate(currently on go) Dockerfile.
4. Run the deployment with ports (need for the first time)
5. Extra: You can get the logs from the application with one of the API (SSE)


### Current Issues
1. Dangling Images are created when image creation.
2. Improve Log Format and retrive from the very start(container create)

## Importance

Please use a proper license for the docker backend.