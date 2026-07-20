FROM scratch
EXPOSE 3000
#ENV RM_ADMIN_API_TOKEN=""
COPY dist/rmfakecloud-docker .
ENTRYPOINT ["/rmfakecloud-docker"]
