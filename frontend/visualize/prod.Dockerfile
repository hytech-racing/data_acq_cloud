FROM node:18-alpine

WORKDIR /react-docker/

COPY public/ /react-docker/public
COPY src/ /react-docker/src
COPY package.json /react-docker/

RUN npm install
RUN npm install -g serve
RUN npm run build

CMD ["serve", "-s", "build"]



