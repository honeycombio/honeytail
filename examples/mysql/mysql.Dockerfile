FROM mysql

COPY my.cnf /etc/mysql/my.cnf
RUN touch /var/log/slowquery.log
RUN chown mysql:mysql /var/log/slowquery.log
