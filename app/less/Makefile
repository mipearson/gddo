BOOTSTRAP=/Users/gary/src/bootstrap/less

default: 
	echo $(BOOTSTRAP)
	#recess --compile  bootstrap.less > ../static/css/bootstrap.css
	#recess --compile -I$(BOOTSTRAP) bootstrap.less > ../static/css/bootstrap.css
	lessc -I$(BOOTSTRAP) bootstrap.less > ../static/css/bootstrap.css
	#lessc -I$(BOOTSTRAP) --comppress bootstrap.less > ../static/css/bootstrap.min.css
