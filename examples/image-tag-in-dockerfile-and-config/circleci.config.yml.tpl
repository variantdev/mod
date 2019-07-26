version: 2
jobs:
  build:
    docker:
      - image: circleci/golang:{{.version}}
    steps:
      - checkout
      - run: build .
