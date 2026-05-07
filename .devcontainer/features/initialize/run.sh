
# Prepare package
rm /etc/apt/sources.list.d/yarn.list
apt-get update -y

## npm
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash
export NVM_DIR="/usr/local/share/nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"  # This loads nvm
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"  # This loads nvm bash_completion
nvm install --lts && nvm use --lts

## pyenv
curl -fsSL https://pyenv.run | bash
cat << 'EOF' >> ~/.bashrc
### pyenv
export PYENV_ROOT="$HOME/.pyenv"
[[ -d $PYENV_ROOT/bin ]] && export PATH="$PYENV_ROOT/bin:$PATH"
eval "$(pyenv init - bash)"
EOF
source ~/.bashrc
apt-get update && apt-get install -y \
build-essential \
sqlite3 \
libsqlite3-dev \
libffi-dev
pyenv install 3.11.9 && pyenv global 3.11.9

## crawl4ai
pip install crawl4ai
crawl4ai-setup && crawl4ai-doctor