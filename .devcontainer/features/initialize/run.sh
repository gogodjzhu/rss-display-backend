
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
# Load pyenv automatically by appending
# the following to
# ~/.bash_profile if it exists, otherwise ~/.profile (for login shells)
# and ~/.bashrc (for interactive shells) :

export PYENV_ROOT="$HOME/.pyenv"
[[ -d $PYENV_ROOT/bin ]] && export PATH="$PYENV_ROOT/bin:$PATH"
eval "$(pyenv init - bash)"

# Restart your shell for the changes to take effect.

# Load pyenv-virtualenv automatically by adding
# the following to ~/.bashrc:

eval "$(pyenv virtualenv-init -)"
EOF
source ~/.bashrc
sudo apt update &&sudo apt install -y \
build-essential \
libssl-dev \
zlib1g-dev \
libbz2-dev \
libreadline-dev \
libsqlite3-dev \
libffi-dev \
libncurses5-dev \
libncursesw5-dev \
liblzma-dev \
tk-dev \
xz-utils
pyenv install 3.11.9 && pyenv global 3.11.9

## crawl4ai
pip install crawl4ai
crawl4ai-setup && crawl4ai-doctor