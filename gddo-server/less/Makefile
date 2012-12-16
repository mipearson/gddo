BOOTSTRAP=$(HOME)/src/bootstrap

default: ../static/bootstrap.min.js
	echo $(BOOTSTRAP)
	lessc -I$(BOOTSTRAP)/less bootstrap.less > ../static/css/bootstrap.css

../static/bootstrap.min.js: $(BOOTSTRAP)/bootstrap/js/bootstrap.min.js
	cp $< $@

#../static/img/glyphicons-halflings.png: $(BOOTSTRAP)/bootstrap/img/glyphicons-halflings.png
#	cp $< $@
#
#../static/img/glyphicons-halflings-white.png: $(BOOTSTRAP)/bootstrap/img/glyphicons-halflings-white.png
#	cp $< $@
