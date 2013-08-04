This project is the source for http://godoc.org/

The code in this project is designed to be used by godoc.org. Send mail to
info@godoc.org if you want to discuss other uses of the code.

Feedback
--------

Send ideas and questions to info@godoc.org. Request features and report bugs
using the [Github Issue
Tracker](https://github.com/garyburd/gopkgdoc/issues/new).


Contributing
------------

Contributions are welcome.

Before writing code, send mail to info@godoc.org to discuss what you plan to
do. This gives the project manager a chance to validate the design, avoid
duplication of effort and ensure that the changes fit the goals of the project.
Do not start the discussion with a pull request.

Development Environment Setup
-----------------------------

- Install and run [Redis 2.6.x](http://redis.io/download). The redis.conf file included in the Redis distribution is suitable for development.
- Install Go from source and update to tip.
- Create an empty `secrets.json` (which can be filled in later):

        echo '{}' > secrets.json

- Install and run the server:

        go get github.com/garyburd/gddo/gddo-server
        gddo-server

- Go to http://localhost:8080/ in your browser
- Enter an import path to have the server retrieve & display a package's documentation

Bootstrap Compilation Setup
---------------------------

godoc uses Bootstrap 3 for its stylesheets & javascript. A compiled &
minified .css & .js are included with this checkout, but if you'd like
to make changes to the stylesheets, you'll need to recompile them yourself.

- Install [bootstrap-3.0.0-RC1](http://http://getbootstrap.com/) by running `bower install`
- Install [recess](https://github.com/twitter/recess) by running `npm install -g recess`


License
-------

[Apache License, Version 2.0](http://www.apache.org/licenses/LICENSE-2.0.html).
