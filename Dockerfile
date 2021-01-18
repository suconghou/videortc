FROM alpine
ADD videortc /
ENTRYPOINT ["/videortc"]
EXPOSE 6060