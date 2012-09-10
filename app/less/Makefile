BOOTSTRAP=/Users/gary/src/bootstrap

default: ../static/bootstrap.min.js
	echo $(BOOTSTRAP)
	lessc -I$(BOOTSTRAP)/less bootstrap.less > ../static/css/bootstrap.css

../static/bootstrap.min.js: $(BOOTSTRAP)/bootstrap/js/bootstrap.min.js
	cp $< $@
