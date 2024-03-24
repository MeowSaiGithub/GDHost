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

### Current Issues
1. Dangling Images are created when image creation.
2. Improve Log Format and retrive from the very start(container create)

## Importance

Please use a proper license for the docker backend.