FROM nginx:alpine

# Create this so honeytail won't crash due to it not existing the first time
# it's brought up.
RUN mkdir -p /var/log/honeytail
RUN touch /var/log/honeytail/access.log

COPY nginx.conf /etc/nginx/nginx.conf
