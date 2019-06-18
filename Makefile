SERVER=aether
SRC_PATH=.
DEST_PATH=~/

deploy:
	rsync -azP ${SRC_PATH} ${SERVER}:${DEST_PATH} --exclude .git/
