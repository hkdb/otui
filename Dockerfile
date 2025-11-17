#
#  OTUI Container
#

FROM ubuntu:24.04

LABEL org.opencontainers.image.maintainer="hkdb <hkdb@3df.io>"
LABEL org.opencontainers.image.source="https://github.com/hkdb/otui"
LABEL org.opencontainers.image.licenses="Apache-2.0"

# Set ENV to Non-Interactive Install
ENV DEBIAN_FRONTEND=noninteractive

# Make sure Ubuntu is up-to-date
RUN apt-get update -y \
    && apt-get install -y apt-utils software-properties-common apt-transport-https

# Use official Go package from standard repositories
RUN apt-get install -y golang

# Install other required packages
RUN apt-get install -y gnupg git curl libnotify4 libnss3 build-essential nano neovim emacs sudo python3 python3-pip python3-venv

# Clean up after installation
RUN apt clean && rm -rf /var/lib/apt/lists/*

# Create user and set ownership
RUN usermod -l otui ubuntu
RUN groupmod -n otui ubuntu
RUN mv /home/ubuntu /home/otui
RUN usermod -d /home/otui otui

# Set Environment
ENV HOME=/home/otui
USER otui
WORKDIR $HOME

# Install Node.js
RUN curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh | bash
ENV NVM_DIR=/home/otui/.nvm
ENV NODE_VERSION=22.21.1
RUN . "$NVM_DIR/nvm.sh" && \
    nvm install $NODE_VERSION && \
    nvm alias default $NODE_VERSION && \
    nvm use default
ENV PATH="$NVM_DIR/versions/node/v${NODE_VERSION}/bin:${PATH}"
RUN mkdir -p /home/otui/.local/bin
RUN mkdir -p /home/otui/.local/share/otui
RUN mkdir -p /home/otui/.config/otui
RUN echo "PATH=\$HOME/.local/bin:\$PATH" > ~/.profile
# Copy otui binary to user's bin directory
COPY ./otui /home/otui/.local/bin/

# Set volume permissions 
USER root
COPY docker/entrypoint.sh /
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
